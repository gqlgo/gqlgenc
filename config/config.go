package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"

	"github.com/99designs/gqlgen/codegen/config"

	"github.com/gqlgo/gqlgenc/clientv2"
	"github.com/gqlgo/gqlgenc/internal/fileglob"
	"github.com/gqlgo/gqlgenc/introspection"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// Config extends the gqlgen basic config
// and represents the config file
type Config struct {
	SchemaFilename StringList           `yaml:"schema,omitempty"`
	Model          config.PackageConfig `yaml:"model,omitempty"`
	AutoBind       []string             `yaml:"autobind"`
	Client         config.PackageConfig `yaml:"client,omitempty"`
	Federation     config.PackageConfig `yaml:"federation,omitempty"`
	Models         config.TypeMap       `yaml:"models,omitempty"`
	Endpoint       *EndPointConfig      `yaml:"endpoint,omitempty"`
	Generate       *GenerateConfig      `yaml:"generate,omitempty"`

	Query []string `yaml:"query"`

	// gqlgen config struct
	GQLConfig *config.Config `yaml:"-"`
}

var cfgFilenames = []string{".gqlgenc.yml", "gqlgenc.yml", "gqlgenc.yaml"}

// StringList is a simple array of strings
type StringList []string

// Has checks if the strings array has a give value
func (a StringList) Has(file string) bool {
	return slices.Contains(a, file)
}

// LoadConfigFromDefaultLocations looks for a config file in the specified directory, and all parent directories
// walking up the tree. The closest config file will be returned.
func LoadConfigFromDefaultLocations(dir string) (*Config, error) {
	cfgFile, err := FindConfigFile(dir)
	if err != nil {
		return nil, err
	}

	return LoadConfig(cfgFile)
}

// FindConfigFile looks for a config file in the specified directory, and all
// parent directories walking up the tree, and returns the path of the closest
// one.
//
// Arguments:
//   - dir: the directory to start from; "." means the current directory
//
// Returns:
//   - string: the path of the config file, joined with dir when dir is not "."
//   - error: non-nil when dir does not exist or no config file is found
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the returned path refers to an existing file named one of .gqlgenc.yml,
//     gqlgenc.yml or gqlgenc.yaml
func FindConfigFile(dir string) (string, error) {
	cfgFile, err := findCfg(dir)
	if err != nil {
		return "", fmt.Errorf("not found Config. Config could not be found. Please make sure the name of the file is correct. want={.gqlgenc.yml, gqlgenc.yml, gqlgenc.yaml}, got=%s: %w", dir, err)
	}

	return cfgFile, nil
}

// EndPointConfig are the allowed options for the 'endpoint' config
type EndPointConfig struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// findCfg searches for the config file in this directory and all parents up the tree
// looking for the closest match
func findCfg(path string) (string, error) {
	var (
		err error
		dir string
	)

	if path == "." {
		dir, err = os.Getwd()
	} else {
		dir = path
		_, err = os.Stat(dir)
	}

	if err != nil {
		return "", fmt.Errorf("unable to get directory \"%s\" to findCfg: %w", dir, err)
	}

	cfg := findCfgInDir(dir)

	for cfg == "" && dir != filepath.Dir(dir) {
		dir = filepath.Dir(dir)
		cfg = findCfgInDir(dir)
	}

	if cfg == "" {
		return "", os.ErrNotExist
	}

	return cfg, nil
}

func findCfgInDir(dir string) string {
	for _, cfgName := range cfgFilenames {
		path := filepath.Join(dir, cfgName)

		_, err := os.Stat(path)
		if err == nil {
			return path
		}
	}

	return ""
}

// LoadConfig loads and parses the config gqlgenc config
func LoadConfig(filename string) (*Config, error) {
	cfg, err := readConfig(filename)
	if err != nil {
		return nil, err
	}

	err = cfg.checkSchemaSource()
	if err != nil {
		return nil, err
	}

	files, err := fileglob.Expand(cfg.SchemaFilename)
	if err != nil {
		return nil, err
	}

	if len(files) > 0 {
		cfg.SchemaFilename = files
	}

	sources, err := readSchemaSources(cfg.SchemaFilename)
	if err != nil {
		return nil, err
	}

	if cfg.Generate == nil {
		cfg.Generate = &GenerateConfig{}
	}

	cfg.Generate.applyDefaults()

	cfg.GQLConfig = cfg.newGQLConfig(sources)

	err = cfg.Client.Check()
	if err != nil {
		return nil, fmt.Errorf("config.exec: %w", err)
	}

	return cfg, nil
}

// readConfig reads and decodes a gqlgenc config file.
//
// Arguments:
//   - filename: the path of the YAML file
//
// Returns:
//   - *Config: the decoded config, without defaults applied
//   - error: non-nil if the file cannot be read or contains unknown or malformed settings
//
// Preconditions:
//   - none
//
// Postconditions:
//   - environment variables referenced in the file are expanded before decoding
func readConfig(filename string) (*Config, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	confContent := []byte(os.ExpandEnv(string(b)))

	decoder := yaml.NewDecoder(bytes.NewReader(confContent), yaml.DisallowUnknownField())

	var cfg Config

	err = decoder.Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	return &cfg, nil
}

// readSchemaSources reads the schema files into parser sources.
//
// Arguments:
//   - filenames: the schema files, already expanded from globs
//
// Returns:
//   - []*ast.Source: one source per file, named by the slash-separated path; empty but not nil for no files
//   - error: non-nil if a file cannot be read
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the sources keep the order of filenames
func readSchemaSources(filenames StringList) ([]*ast.Source, error) {
	sources := []*ast.Source{}

	for _, filename := range filenames {
		filename = filepath.ToSlash(filename)

		schemaRaw, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		sources = append(sources, &ast.Source{Name: filename, Input: string(schemaRaw)})
	}

	return sources, nil
}

// LoadSchema load and parses the schema from a local file or a remote server
func (c *Config) LoadSchema(ctx context.Context) error {
	var schema *ast.Schema

	switch source := c.schemaSource(); source {
	case schemaSourceLocal, schemaSourceAmbiguous:
		// LoadConfig rejects an ambiguous source. A config built by hand with
		// both settings has always been loaded from the local files.
		s, err := c.loadLocalSchema()
		if err != nil {
			return fmt.Errorf("load local schema failed: %w", err)
		}

		schema = s
	case schemaSourceRemote:
		s, err := c.loadRemoteSchema(ctx)
		if err != nil {
			return fmt.Errorf("load remote schema failed: %w", err)
		}

		schema = s
	case schemaSourceMissing:
		return errSchemaSourceMissing
	default:
		panic(fmt.Sprintf("unknown schema source %d", source))
	}

	if schema.Query == nil {
		schema.Query = &ast.Definition{
			Kind: ast.Object,
			Name: "Query",
		}
		schema.Types["Query"] = schema.Query
	}

	c.GQLConfig.Schema = schema

	return nil
}

func (c *Config) loadRemoteSchema(ctx context.Context) (*ast.Schema, error) {
	addHeaderInterceptor := func(ctx context.Context, req *http.Request, gqlInfo *clientv2.GQLRequestInfo, res any, next clientv2.RequestInterceptorFunc) error {
		for key, value := range c.Endpoint.Headers {
			req.Header.Set(key, value)
		}

		return next(ctx, req, gqlInfo, res)
	}

	gqlclient := clientv2.NewClient(http.DefaultClient, c.Endpoint.URL, nil, addHeaderInterceptor)

	var res introspection.Query

	err := gqlclient.Post(ctx, "Query", introspection.NewQuery(remoteCapabilities(ctx, gqlclient)), &res, nil)
	if err != nil {
		return nil, fmt.Errorf("introspection query failed: %w", err)
	}

	schema, err := validator.ValidateSchemaDocument(introspection.ParseIntrospectionQuery(c.Endpoint.URL, res))
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	return schema, nil
}

// remoteCapabilities asks the server which optional parts of introspection it supports.
//
// Arguments:
//   - ctx: the context of the request
//   - gqlclient: the client for the endpoint
//
// Returns:
//   - introspection.Capabilities: the supported parts, or the zero value when
//     the server does not answer the capability query
//
// Preconditions:
//   - gqlclient is non-nil
//
// Postconditions:
//   - a failed capability query is not reported; the introspection query that
//     follows uses no optional part and reports the failure if it persists
func remoteCapabilities(ctx context.Context, gqlclient *clientv2.Client) introspection.Capabilities {
	var capabilityQuery introspection.CapabilityQuery

	err := gqlclient.Post(ctx, "Capabilities", introspection.CapabilityQueryDocument, &capabilityQuery, nil)
	if err != nil {
		return introspection.Capabilities{}
	}

	return capabilityQuery.Capabilities()
}

func (c *Config) loadLocalSchema() (*ast.Schema, error) {
	schema, err := gqlparser.LoadSchema(c.GQLConfig.Sources...)
	if err != nil {
		return nil, fmt.Errorf("loadLocalSchema: %w", err)
	}

	return schema, nil
}

// checkSchemaSource verifies that exactly one of schema and endpoint is set.
//
// Arguments:
//   - none
//
// Returns:
//   - error: non-nil if both or neither of the schema and endpoint settings are present
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (c *Config) checkSchemaSource() error {
	switch source := c.schemaSource(); source {
	case schemaSourceAmbiguous:
		return errSchemaSourceAmbiguous
	case schemaSourceMissing:
		return errSchemaSourceMissing
	case schemaSourceLocal, schemaSourceRemote:
		return nil
	default:
		panic(fmt.Sprintf("unknown schema source %d", source))
	}
}

// schemaSource is where the schema of a config comes from. The config file
// expresses it with two optional settings, 'schema' and 'endpoint', of which
// exactly one must be present.
type schemaSource int

const (
	// schemaSourceLocal means only 'schema' is set: the schema is read from local files.
	schemaSourceLocal schemaSource = iota + 1
	// schemaSourceRemote means only 'endpoint' is set: the schema is introspected from a server.
	schemaSourceRemote
	// schemaSourceAmbiguous means both 'schema' and 'endpoint' are set.
	schemaSourceAmbiguous
	// schemaSourceMissing means neither 'schema' nor 'endpoint' is set.
	schemaSourceMissing
)

var (
	errSchemaSourceAmbiguous = errors.New("'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	errSchemaSourceMissing   = errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
)

// schemaSource reports where the schema of the config comes from.
//
// Arguments:
//   - none
//
// Returns:
//   - schemaSource: decided by which of SchemaFilename and Endpoint are non-nil
//
// Preconditions:
//   - none
//
// Postconditions:
//   - none
func (c *Config) schemaSource() schemaSource {
	hasLocal := c.SchemaFilename != nil
	hasRemote := c.Endpoint != nil

	switch {
	case hasLocal && hasRemote:
		return schemaSourceAmbiguous
	case hasLocal:
		return schemaSourceLocal
	case hasRemote:
		return schemaSourceRemote
	default:
		return schemaSourceMissing
	}
}

// newGQLConfig projects the gqlgenc settings onto the gqlgen config used for generation.
//
// Arguments:
//   - sources: the schema sources
//
// Returns:
//   - *config.Config: the gqlgen config with the model, autobind, and generation settings applied
//
// Preconditions:
//   - c.Generate is set and has its defaults applied
//
// Postconditions:
//   - the returned config is not yet initialized; call Init after loading the schema
//   - model JSON-tag pointers are the configured model values, or a copy of the
//     client values when the model values were left nil
func (c *Config) newGQLConfig(sources []*ast.Source) *config.Config {
	models := make(config.TypeMap)
	if c.Models != nil {
		models = c.Models
	}

	// A nil model tag falls back to the client tag, copied into a new pointer
	// so the two settings do not alias.
	modelOmitempty := c.Generate.EnableModelJsonOmitemptyTag
	if modelOmitempty == nil {
		modelOmitempty = boolPointer(*c.Generate.EnableClientJsonOmitemptyTag)
	}

	modelOmitzero := c.Generate.EnableModelJsonOmitzeroTag
	if modelOmitzero == nil {
		modelOmitzero = boolPointer(*c.Generate.EnableClientJsonOmitzeroTag)
	}

	return &config.Config{
		Model:    c.Model,
		Models:   models,
		AutoBind: c.AutoBind,
		// TODO: gqlgen must be set exec but client not used
		Exec:                           config.ExecConfig{Filename: "generated.go"},
		Directives:                     map[string]config.DirectiveConfig{},
		Sources:                        sources,
		StructFieldsAlwaysPointers:     *c.Generate.StructFieldsAlwaysPointers,
		ReturnPointersInUnmarshalInput: false,
		ResolversAlwaysReturnPointers:  true,
		NullableInputOmittable:         c.Generate.NullableInputOmittable,
		EnableModelJsonOmitemptyTag:    modelOmitempty,
		EnableModelJsonOmitzeroTag:     modelOmitzero,
	}
}
