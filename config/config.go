// Package config は、設定ファイル (.gqlgenc.yml) を読み込む。
//
// Config は設定の値だけを持つ。スキーマ・クエリ・Go の型の束縛など、設定から読み込んだ結果は
// 持たず、読み込みは呼び出し側 (main) が順に行う。
package config

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// errNoSchemaSource is returned when neither a local schema nor a remote
// endpoint is configured.
var errNoSchemaSource = errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")

// Config は、設定ファイルから読み込んだ設定の値。LoadConfig が fileConfig から作る。
// YAML の形 (fileConfig) とは対応せず、生成の各段階が使う形にしてある。
type Config struct {
	// SchemaFiles は schema.files のグロブを展開した、スキーマのファイルの一覧 (名前順)。
	// Endpoint とは、どちらか一方だけを持つ。
	SchemaFiles []string
	// Endpoint は schema.endpoint。スキーマを introspection で取得するサーバー。
	Endpoint *EndPointConfig
	// FederationVersion は schema.federation.version。0 なら Apollo Federation のディレクティブを
	// スキーマへ足さない。
	FederationVersion int

	// Query は query.files。クエリのファイルのグロブ。
	Query []string

	// TypeAutobind は bind.type.packages。スキーマの型を、同じ名前の Go 型へ束縛するパッケージ。
	TypeAutobind []string
	// Models は bind.type.named。スキーマの型の名前ごとの、束縛先の Go 型 ("import/path.TypeName" の形)。
	Models map[string]string
	// FragmentAutobind は bind.fragment.packages。フラグメントを、同じ名前の Go 型へ束縛するパッケージ。
	FragmentAutobind []string

	// ModelGen は generate.model.file の出力先。省略できる (IsDefined が false)。
	ModelGen PackageConfig
	// ModelOnlyUsed は、クエリで使われている Input / Enum 型だけを生成するか (デフォルト true)。
	// false のときはスキーマの全 Input / Enum 型を生成する。
	ModelOnlyUsed bool
	// QueryGen は generate.query.file の出力先。省略できる。
	QueryGen PackageConfig
	// GenerateGetters は generate.query.getters。レスポンスの型に getter を生成するか。
	GenerateGetters bool
	// ClientGen は generate.client.file の出力先。省略できる。
	ClientGen PackageConfig
}

// fileConfig is the on-disk .gqlgenc.yml schema. gqlgenc owns this schema and
// translates it into Config; the raw gqlgen config is not exposed. The gqlgen
// settings that v3 always requires (json/v2 tags, etc.) are fixed internally.
type fileConfig struct {
	Schema   schemaConfig   `yaml:"schema"`
	Query    queryConfig    `yaml:"query"`
	Bind     bindConfig     `yaml:"bind,omitempty"`
	Generate generateConfig `yaml:"generate"`
}

// schemaConfig is the schema source: local files or a remote endpoint (exactly
// one), plus the Apollo Federation setting that shapes how the schema parses.
type schemaConfig struct {
	Files      []string          `yaml:"files,omitempty"`
	Endpoint   *EndPointConfig   `yaml:"endpoint,omitempty"`
	Federation *federationConfig `yaml:"federation,omitempty"`
}

// federationConfig enables Apollo Federation schema directives of the given version.
type federationConfig struct {
	Version int `yaml:"version"`
}

// queryConfig is the query source files.
type queryConfig struct {
	Files []string `yaml:"files,omitempty"`
}

// bindConfig binds GraphQL names to existing Go types: type binds schema types,
// fragment binds query fragments. Bindings affect all generation (models and
// query response types), so they are kept separate from the generation outputs.
type bindConfig struct {
	Type     typeBindConfig     `yaml:"type,omitempty"`
	Fragment fragmentBindConfig `yaml:"fragment,omitempty"`
}

// typeBindConfig binds schema types: packages binds type names to same-named Go
// types in the listed packages, named binds individual GraphQL types to Go types.
type typeBindConfig struct {
	Packages []string                   `yaml:"packages,omitempty"`
	Named    map[string]namedTypeConfig `yaml:"named,omitempty"`
}

// namedTypeConfig は、bind.type.named の GraphQL の型 1 つの束縛先。
type namedTypeConfig struct {
	// Model は、束縛先の Go 型 ("import/path.TypeName" の形)
	Model string `yaml:"model"`
}

// fragmentBindConfig binds fragment names to same-named Go types in the listed packages.
type fragmentBindConfig struct {
	Packages []string `yaml:"packages,omitempty"`
}

// generateConfig lists the files to generate. model is optional: when set the
// query's input and enum types are generated, when omitted they are shared via
// bind.type.packages. The package of each file is inferred from its directory.
type generateConfig struct {
	Model  generateModelConfig  `yaml:"model,omitempty"`
	Query  generateQueryConfig  `yaml:"query,omitempty"`
	Client generateClientConfig `yaml:"client,omitempty"`
}

// generateModelConfig is the generated input/enum model output. onlyUsed
// (default true) restricts generation to the input/enum types the queries
// actually use; set false to generate every input/enum type in the schema.
type generateModelConfig struct {
	File     string `yaml:"file,omitempty"`
	OnlyUsed *bool  `yaml:"onlyUsed,omitempty"`
}

// generateQueryConfig is the generated query_gen.go output: the response types
// (file) and the getter toggle (which applies to all types generated into the
// file, fragment types included).
type generateQueryConfig struct {
	File    string `yaml:"file,omitempty"`
	Getters bool   `yaml:"getters,omitempty"`
}

// generateClientConfig is the generated typed client output.
type generateClientConfig struct {
	File string `yaml:"file,omitempty"`
}

// LoadConfig reads, parses and validates a .gqlgenc.yml file and translates it
// into Config.
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
	if len(fc.Query.Files) == 0 {
		return nil, errors.New("'query.files' is required")
	}
	if fc.Generate.Query.File == "" && fc.Generate.Model.File == "" {
		return nil, errors.New("neither 'generate.query.file' nor 'generate.model.file' specified, at least one generation target is required")
	}
	if fc.Generate.Client.File != "" && fc.Generate.Query.File == "" {
		return nil, errors.New("'generate.client.file' is set, 'generate.query.file' must be set")
	}

	var models map[string]string
	for _, typeName := range slices.Sorted(maps.Keys(fc.Bind.Type.Named)) {
		model := fc.Bind.Type.Named[typeName].Model
		if model == "" {
			return nil, fmt.Errorf("'bind.type.named.%s.model' is required", typeName)
		}
		if models == nil {
			models = make(map[string]string, len(fc.Bind.Type.Named))
		}
		models[typeName] = model
	}

	var federationVersion int
	if fc.Schema.Federation != nil {
		federationVersion = fc.Schema.Federation.Version
	}

	c := Config{
		SchemaFiles:       nil,
		Endpoint:          fc.Schema.Endpoint,
		FederationVersion: federationVersion,
		Query:             fc.Query.Files,
		TypeAutobind:      fc.Bind.Type.Packages,
		Models:            models,
		FragmentAutobind:  fc.Bind.Fragment.Packages,
		ModelGen:          PackageConfig{},
		ModelOnlyUsed:     fc.Generate.Model.OnlyUsed == nil || *fc.Generate.Model.OnlyUsed,
		QueryGen:          PackageConfig{},
		GenerateGetters:   fc.Generate.Query.Getters,
		ClientGen:         PackageConfig{},
	}

	// 出力先は、ファイル名を絶対パスにし、パッケージ名と import パスをディレクトリから決める。
	// model はサーバー側 (gqlgen) で生成済みのモデルを autobind で使う場合に省略できる
	outputs := []struct {
		key      string
		filename string
		output   *PackageConfig
	}{
		{key: "generate.model.file", filename: fc.Generate.Model.File, output: &c.ModelGen},
		{key: "generate.query.file", filename: fc.Generate.Query.File, output: &c.QueryGen},
		{key: "generate.client.file", filename: fc.Generate.Client.File, output: &c.ClientGen},
	}
	for _, o := range outputs {
		*o.output, err = newPackageConfig(o.filename)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.key, err)
		}
	}

	if hasFiles {
		c.SchemaFiles, err = schemaFilenames(fc.Schema.Files)
		if err != nil {
			return nil, err
		}
		// グロブがどのファイルにも一致しなければ、スキーマを読み込む先が無い
		if len(c.SchemaFiles) == 0 {
			return nil, errNoSchemaSource
		}
	}

	return &c, nil
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
