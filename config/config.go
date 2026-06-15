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

// Config represents the config file.
type Config struct {
	GQLGencConfig *GQLGencConfig       `yaml:"gqlgenc"`
	GQLGenConfig  *gqlgenconfig.Config `yaml:"gqlgen"`
}

// LoadConfig loads and parses the config gqlgenc config.
func LoadConfig(configFilename string) (*Config, error) {
	configContent, err := os.ReadFile(configFilename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	var c Config

	yamlDecoder := yaml.NewDecoder(strings.NewReader(os.ExpandEnv(string(configContent))), yaml.DisallowUnknownField())
	if err := yamlDecoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	// validation
	if c.GQLGencConfig == nil {
		return nil, errors.New("'gqlgenc' section is not specified")
	}
	if c.GQLGenConfig == nil {
		return nil, errors.New("'gqlgen' section is not specified")
	}

	if c.GQLGenConfig.SchemaFilename != nil && c.GQLGencConfig.Endpoint != nil {
		return nil, errors.New("'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	if c.GQLGenConfig.SchemaFilename == nil && c.GQLGencConfig.Endpoint == nil {
		return nil, errNoSchemaSource
	}

	if c.GQLGencConfig.ClientGen.IsDefined() && !c.GQLGencConfig.QueryGen.IsDefined() {
		return nil, errors.New("'clientgen' is set, 'querygen' must be set")
	}

	if !c.GQLGenConfig.Model.IsDefined() && !c.GQLGencConfig.QueryGen.IsDefined() {
		return nil, errors.New("neither 'model' nor 'querygen' specified, at least one generation target is required")
	}

	///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgen

	// model はサーバー側 (gqlgen) で生成済みのモデルを autobind で使う場合に省略できる
	if c.GQLGenConfig.Model.IsDefined() {
		if err := c.GQLGenConfig.Model.Check(); err != nil {
			return nil, fmt.Errorf("model: %w", err)
		}
	}

	// Fill gqlgen config fields
	// https://github.com/99designs/gqlgen/blob/3a31a752df764738b1f6e99408df3b169d514784/codegen/config/config.go#L120
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

	///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgenc

	// validation
	if c.GQLGencConfig.QueryGen.IsDefined() {
		if err := c.GQLGencConfig.QueryGen.Check(); err != nil {
			return nil, fmt.Errorf("querygen: %w", err)
		}
	}

	if c.GQLGencConfig.ClientGen.IsDefined() {
		if err := c.GQLGencConfig.ClientGen.Check(); err != nil {
			return nil, fmt.Errorf("clientgen: %w", err)
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

	// model を生成しない場合は modelgen が動かないため、未バインドの custom scalar を
	// modelgen と同じ既定 (graphql.String) に束縛する。built-in scalar と models: で
	// 明示バインドした型は UserDefined のため対象外になる。
	if !c.GQLGenConfig.Model.IsDefined() {
		for _, t := range c.GQLGenConfig.Schema.Types {
			if t.Kind == ast.Scalar && !c.GQLGenConfig.Models.UserDefined(t.Name) {
				c.GQLGenConfig.Models.Add(t.Name, "github.com/99designs/gqlgen/graphql.String")
			}
		}
	}

	// sort Implements to ensure a deterministic output
	for _, implements := range c.GQLGenConfig.Schema.Implements {
		slices.SortFunc(implements, func(a, b *ast.Definition) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	return nil
}

type GQLGencConfig struct {
	QueryGen                gqlgenconfig.PackageConfig `yaml:"querygen,omitempty"`
	ClientGen               gqlgenconfig.PackageConfig `yaml:"clientgen,omitempty"`
	Endpoint                *EndPointConfig            `yaml:"endpoint,omitempty"`
	Query                   []string                   `yaml:"query"`
	Autobind                []string                   `yaml:"autobind,omitempty"`
	ExportQueryType         bool                       `yaml:"export_query_type,omitempty"`
	GenerateGetters         bool                       `yaml:"generate_getters,omitempty"`
	QueryDocument           *ast.QueryDocument         `yaml:"-"`
	OperationQueryDocuments []*ast.QueryDocument       `yaml:"-"`
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
