package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin/federation"

	"github.com/Yamashou/gqlgenc/v3/queryparser"

	"github.com/vektah/gqlparser/v2/ast"
)

// errNoSchemaSource is returned when neither a local schema nor a remote
// endpoint is configured.
var errNoSchemaSource = errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")

// Config is gqlgenc's internal configuration: the gqlgenc-specific settings plus
// the gqlgen config they drive. It is built from the on-disk fileConfig by
// LoadConfig; the YAML file no longer maps onto it directly.
type Config struct {
	GQLGencConfig *GQLGencConfig
	GQLGenConfig  *gqlgenconfig.Config
}

// fileConfig is the on-disk .gqlgenc.yml schema. gqlgenc owns this schema and
// translates it into Config; the raw gqlgen config is not exposed. The gqlgen
// settings that v3 always requires (json/v2 tags, etc.) are fixed internally.
type fileConfig struct {
	Schema     schemaConfig         `yaml:"schema"`
	Queries    []string             `yaml:"queries"`
	Generate   generateConfig       `yaml:"generate"`
	Autobind   autobindConfig       `yaml:"autobind,omitempty"`
	Models     gqlgenconfig.TypeMap `yaml:"models,omitempty"`
	Federation *federationConfig    `yaml:"federation,omitempty"`
	Options    optionsConfig        `yaml:"options,omitempty"`
}

// federationConfig enables Apollo Federation schema directives of the given version.
type federationConfig struct {
	Version int `yaml:"version"`
}

// schemaConfig is the schema source: local files or a remote endpoint (exactly one).
type schemaConfig struct {
	Files    []string        `yaml:"files,omitempty"`
	Endpoint *EndPointConfig `yaml:"endpoint,omitempty"`
}

// generateConfig lists the files to generate. model is optional: when set the
// query's input and enum types are generated, when omitted they are shared via
// autobind. The package of each file is inferred from its directory.
type generateConfig struct {
	Query  string `yaml:"query,omitempty"`
	Client string `yaml:"client,omitempty"`
	Model  string `yaml:"model,omitempty"`
}

// autobindConfig binds GraphQL names to existing Go types: model binds schema
// type names, fragment binds query fragment names.
type autobindConfig struct {
	Model    []string `yaml:"model,omitempty"`
	Fragment []string `yaml:"fragment,omitempty"`
}

// optionsConfig holds output toggles.
type optionsConfig struct {
	Getters bool `yaml:"getters,omitempty"`
}

// LoadConfig reads, parses and validates a .gqlgenc.yml file and translates it
// into the internal Config (gqlgenc settings + the gqlgen config they drive).
func LoadConfig(configFilename string) (*Config, error) {
	configContent, err := os.ReadFile(configFilename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	var fc fileConfig
	yamlDecoder := yaml.NewDecoder(strings.NewReader(os.ExpandEnv(string(configContent))), yaml.DisallowUnknownField())
	if err := yamlDecoder.Decode(&fc); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	// validation
	hasFiles := len(fc.Schema.Files) > 0
	hasEndpoint := fc.Schema.Endpoint != nil
	if hasFiles && hasEndpoint {
		return nil, errors.New("'schema.files' and 'schema.endpoint' both specified. Use files to load from local files, use endpoint to load from a remote server (using introspection)")
	}
	if !hasFiles && !hasEndpoint {
		return nil, errNoSchemaSource
	}
	if len(fc.Queries) == 0 {
		return nil, errors.New("'queries' is required")
	}
	if fc.Generate.Query == "" && fc.Generate.Model == "" {
		return nil, errors.New("neither 'generate.query' nor 'generate.model' specified, at least one generation target is required")
	}
	if fc.Generate.Client != "" && fc.Generate.Query == "" {
		return nil, errors.New("'generate.client' is set, 'generate.query' must be set")
	}

	var federationVersion int
	if fc.Federation != nil {
		federationVersion = fc.Federation.Version
	}

	// translate fileConfig into the internal Config
	c := Config{
		GQLGencConfig: &GQLGencConfig{
			QueryGen:         gqlgenconfig.PackageConfig{Filename: fc.Generate.Query},
			ClientGen:        gqlgenconfig.PackageConfig{Filename: fc.Generate.Client},
			Endpoint:         fc.Schema.Endpoint,
			Query:            fc.Queries,
			FragmentAutobind: fc.Autobind.Fragment,
			GenerateGetters:  fc.Options.Getters,
		},
		GQLGenConfig: &gqlgenconfig.Config{
			SchemaFilename: gqlgenconfig.StringList(fc.Schema.Files),
			Model:          gqlgenconfig.PackageConfig{Filename: fc.Generate.Model},
			AutoBind:       fc.Autobind.Model,
			Models:         fc.Models,
			Federation:     gqlgenconfig.PackageConfig{Version: federationVersion},
			// v3 が常に使う設定を固定する (gqlgen の生 config はユーザに露出しない)。
			// EnableModelJsonOmitemptyTag は未設定 (nil = gqlgen 既定で omitempty 付与) のままにする。
			StructTag:                  "json",
			StructFieldsAlwaysPointers: false,
			NullableInputOmittable:     true,
			EnableModelJsonOmitzeroTag: new(true),
		},
	}

	// model はサーバー側 (gqlgen) で生成済みのモデルを autobind で使う場合に省略できる
	if c.GQLGenConfig.Model.IsDefined() {
		if err := c.GQLGenConfig.Model.Check(); err != nil {
			return nil, fmt.Errorf("generate.model: %w", err)
		}
	}

	// Fill gqlgen config fields
	schemaFilename, err := schemaFilenames(c.GQLGenConfig.SchemaFilename)
	if err != nil {
		return nil, err
	}
	c.GQLGenConfig.SchemaFilename = schemaFilename

	sources, err := schemaFileSources(c.GQLGenConfig.SchemaFilename)
	if err != nil {
		return nil, err
	}

	if c.GQLGenConfig.Federation.Version != 0 {
		fedPlugin, err := federation.New(c.GQLGenConfig.Federation.Version, c.GQLGenConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create federation plugin: %w", err)
		}

		federationSources, err := fedPlugin.InjectSourcesEarly()
		if err != nil {
			return nil, fmt.Errorf("failed to inject federation directives: %w", err)
		}

		sources = append(sources, federationSources...)
	}

	c.GQLGenConfig.Sources = sources

	// gqlgen must be followings parameters
	// クライアント生成では使用しないため固定のダミーファイル名を設定する
	const unusedGenFilename = "generated.go"
	c.GQLGenConfig.Directives = make(map[string]gqlgenconfig.DirectiveConfig)
	c.GQLGenConfig.Exec = gqlgenconfig.ExecConfig{Filename: unusedGenFilename}
	c.GQLGenConfig.Resolver = gqlgenconfig.ResolverConfig{Filename: unusedGenFilename}
	c.GQLGenConfig.Federation = gqlgenconfig.PackageConfig{Filename: unusedGenFilename}

	// gqlgenc validation
	if c.GQLGencConfig.QueryGen.IsDefined() {
		if err := c.GQLGencConfig.QueryGen.Check(); err != nil {
			return nil, fmt.Errorf("generate.query: %w", err)
		}
	}
	if c.GQLGencConfig.ClientGen.IsDefined() {
		if err := c.GQLGencConfig.ClientGen.Check(); err != nil {
			return nil, fmt.Errorf("generate.client: %w", err)
		}
	}

	return &c, nil
}

// RemoteSchemaLoader fetches an AST schema from a GraphQL endpoint via
// introspection. Injecting it lets LoadSchema load a remote schema while
// keeping the config package free of the client dependency.
type RemoteSchemaLoader func(ctx context.Context, endpoint string, header http.Header) (*ast.Schema, error)

func (c *Config) LoadSchema(ctx context.Context, loadRemoteSchema RemoteSchemaLoader) error {
	// Load schema
	switch {
	case c.GQLGenConfig.SchemaFilename != nil:
		if err := c.GQLGenConfig.LoadSchema(); err != nil {
			return fmt.Errorf("load local schema failed: %w", err)
		}
	case c.GQLGencConfig.Endpoint != nil:
		// リモート(endpoint)のスキーマ取得は注入された loadRemoteSchema に委ねる。
		// これにより config パッケージは client / introspection に依存しない。
		schema, err := loadRemoteSchema(ctx, c.GQLGencConfig.Endpoint.URL, http.Header(c.GQLGencConfig.Endpoint.Headers))
		if err != nil {
			return fmt.Errorf("introspect schema failed: %w", err)
		}
		c.GQLGenConfig.Schema = schema
	default:
		return errNoSchemaSource
	}

	// delete exist gen file
	if c.GQLGenConfig.Model.IsDefined() {
		// model gen file must be removed before cfg.PrepareSchema()
		_ = os.Remove(c.GQLGenConfig.Model.Filename)
	}

	if c.GQLGencConfig.QueryGen.IsDefined() {
		_ = os.Remove(c.GQLGencConfig.QueryGen.Filename)
	}

	if c.GQLGencConfig.ClientGen.IsDefined() {
		_ = os.Remove(c.GQLGencConfig.ClientGen.Filename)
	}

	// gqlgen.Config.Init() に必要なフィールドを初期化
	if c.GQLGenConfig.Models == nil {
		c.GQLGenConfig.Models = make(gqlgenconfig.TypeMap)
	}
	if c.GQLGenConfig.StructTag == "" {
		c.GQLGenConfig.StructTag = "json"
	}

	if err := c.GQLGenConfig.Init(); err != nil {
		return fmt.Errorf("generating core failed: %w", err)
	}

	// model を生成しない場合は modelgen が動かないため、custom scalar の既定を補う。
	if !c.GQLGenConfig.Model.IsDefined() {
		bindDefaultScalars(c.GQLGenConfig.Schema, c.GQLGenConfig.Models)
	}

	// sort Implements to ensure a deterministic output
	for _, implements := range c.GQLGenConfig.Schema.Implements {
		slices.SortFunc(implements, func(a, b *ast.Definition) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	return nil
}

// bindDefaultScalars は、まだモデルが束縛されていない schema の custom scalar を
// graphql.String に束縛する。これは gqlgen modelgen が非 user-defined scalar に与える既定
// (plugin/modelgen の b.Scalars 束縛) と揃えたもので、modelgen が動かない (model を生成しない)
// 構成でも同じ既定を適用するために使う。
//
// autobind / models: / built-in で既に束縛済みの scalar は UserDefined のため対象外で、
// 対応する型がある場合はその型へのバインドが優先される。
func bindDefaultScalars(schema *ast.Schema, models gqlgenconfig.TypeMap) {
	for _, t := range schema.Types {
		if t.Kind == ast.Scalar && !models.UserDefined(t.Name) {
			models.Add(t.Name, "github.com/99designs/gqlgen/graphql.String")
		}
	}
}

// GQLGencConfig holds gqlgenc's settings, built by LoadConfig from the on-disk
// fileConfig. It is no longer parsed from YAML directly.
type GQLGencConfig struct {
	QueryGen                gqlgenconfig.PackageConfig
	ClientGen               gqlgenconfig.PackageConfig
	Endpoint                *EndPointConfig
	Query                   []string
	FragmentAutobind        []string
	GenerateGetters         bool
	QueryDocument           *ast.QueryDocument
	OperationQueryDocuments []*ast.QueryDocument
}

func (c *GQLGencConfig) LoadQuery(schema *ast.Schema) error {
	querySources, err := queryparser.LoadQuerySources(c.Query)
	if err != nil {
		return fmt.Errorf("load query sources failed: %w", err)
	}

	queryDocument, err := queryparser.QueryDocument(schema, querySources)
	if err != nil {
		return fmt.Errorf("build query document failed: %w", err)
	}

	operationQueryDocuments, err := queryparser.OperationQueryDocuments(schema, queryDocument.Operations)
	if err != nil {
		return fmt.Errorf("build operation documents failed: %w", err)
	}

	c.QueryDocument = queryDocument
	c.OperationQueryDocuments = operationQueryDocuments

	return nil
}

// EndPointConfig are the allowed options for the 'endpoint' config.
type EndPointConfig struct {
	Headers Header `yaml:"headers,omitempty"`
	URL     string `yaml:"url"`
}

// Header は HTTP ヘッダーの YAML 表現。値には文字列と文字列リストの両方を指定できる。
//
//	headers:
//	  Authorization: "Bearer token"
//	  Accept: ["application/json", "text/plain"]
type Header http.Header

// UnmarshalYAML は文字列スカラーと文字列リストの両方をヘッダー値として受け付ける。
func (h *Header) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}

	result := make(Header, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			result[key] = []string{v}
		case []any:
			values := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("header %q: values must be strings, got %T", key, item)
				}
				values = append(values, s)
			}
			result[key] = values
		default:
			return fmt.Errorf("header %q: value must be a string or a list of strings, got %T", key, value)
		}
	}

	*h = result

	return nil
}
