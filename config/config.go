package config

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/gqlgo/gqlgenc/clientv2"
	"github.com/gqlgo/gqlgenc/introspection"
	"github.com/goccy/go-yaml"
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
	cfgFile, err := findCfg(dir)
	if err != nil {
		return nil, fmt.Errorf("not found Config. Config could not be found. Please make sure the name of the file is correct. want={.gqlgenc.yml, gqlgenc.yml, gqlgenc.yaml}, got=%s: %w", dir, err)
	}

	return LoadConfig(cfgFile)
}

// EndPointConfig are the allowed options for the 'endpoint' config
type EndPointConfig struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// findCfg searches for the config file in this directory and all parents up the tree
// looking for the closest match
func findCfg(path string) (string, error) {
	var err error
	var dir string
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
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

var path2regex = strings.NewReplacer(
	`.`, `\.`,
	`*`, `.+`,
	`\`, `[\\/]`,
	`/`, `[\\/]`,
)

// LoadConfig loads and parses the config gqlgenc config
func LoadConfig(filename string) (*Config, error) {
	var cfg Config
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	confContent := []byte(os.ExpandEnv(string(b)))

	decoder := yaml.NewDecoder(bytes.NewReader(confContent), yaml.DisallowUnknownField())
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	if cfg.SchemaFilename != nil && cfg.Endpoint != nil {
		return nil, fmt.Errorf("'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	if cfg.SchemaFilename == nil && cfg.Endpoint == nil {
		return nil, fmt.Errorf("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	// https://github.com/99designs/gqlgen/blob/3a31a752df764738b1f6e99408df3b169d514784/codegen/config/config.go#L120
	files := StringList{}
	for _, f := range cfg.SchemaFilename {
		var matches []string

		// for ** we want to override default globbing patterns and walk all
		// subdirectories to match schema files.
		if strings.Contains(f, "**") {
			pathParts := strings.SplitN(f, "**", 2)
			rest := strings.TrimPrefix(strings.TrimPrefix(pathParts[1], `\`), `/`)
			// turn the rest of the glob into a regex, anchored only at the end because ** allows
			// for any number of dirs in between and walk will let us match against the full path name
			globRe := regexp.MustCompile(path2regex.Replace(rest) + `$`)

			if err := filepath.Walk(pathParts[0], func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if globRe.MatchString(strings.TrimPrefix(path, pathParts[0])) {
					matches = append(matches, path)
				}

				return nil
			}); err != nil {
				return nil, fmt.Errorf("failed to walk schema at root %s: %w", pathParts[0], err)
			}
		} else {
			matches, err = filepath.Glob(f)
			if err != nil {
				return nil, fmt.Errorf("failed to glob schema filename %s: %w", f, err)
			}
		}

		for _, m := range matches {
			if !files.Has(m) {
				files = append(files, m)
			}
		}
	}

	if len(files) > 0 {
		cfg.SchemaFilename = files
	}

	models := make(config.TypeMap)
	if cfg.Models != nil {
		models = cfg.Models
	}

	sources := []*ast.Source{}

	for _, filename := range cfg.SchemaFilename {
		filename = filepath.ToSlash(filename)
		var err error
		var schemaRaw []byte
		schemaRaw, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		sources = append(sources, &ast.Source{Name: filename, Input: string(schemaRaw)})
	}

	structFieldsAlwaysPointers := true
	enableClientJsonOmitemptyTag := true
	enableModelJsonOmitzeroTag := false
	if cfg.Generate == nil {
		cfg.Generate = &GenerateConfig{
			StructFieldsAlwaysPointers:   &structFieldsAlwaysPointers,
			EnableClientJsonOmitemptyTag: &enableClientJsonOmitemptyTag,
			EnableClientJsonOmitzeroTag:  &enableModelJsonOmitzeroTag,
		}
	}
	if cfg.Generate.StructFieldsAlwaysPointers == nil {
		cfg.Generate.StructFieldsAlwaysPointers = &structFieldsAlwaysPointers
	}
	if cfg.Generate.EnableClientJsonOmitemptyTag == nil {
		cfg.Generate.EnableClientJsonOmitemptyTag = &enableClientJsonOmitemptyTag
	}
	if cfg.Generate.EnableClientJsonOmitzeroTag == nil {
		cfg.Generate.EnableClientJsonOmitzeroTag = &enableModelJsonOmitzeroTag
	}

	cfg.GQLConfig = &config.Config{
		Model:    cfg.Model,
		Models:   models,
		AutoBind: cfg.AutoBind,
		// TODO: gqlgen must be set exec but client not used
		Exec:                           config.ExecConfig{Filename: "generated.go"},
		Directives:                     map[string]config.DirectiveConfig{},
		Sources:                        sources,
		StructFieldsAlwaysPointers:     *cfg.Generate.StructFieldsAlwaysPointers,
		ReturnPointersInUnmarshalInput: false,
		ResolversAlwaysReturnPointers:  true,
		NullableInputOmittable:         cfg.Generate.NullableInputOmittable,
		EnableModelJsonOmitemptyTag:    cfg.Generate.EnableClientJsonOmitemptyTag,
		EnableModelJsonOmitzeroTag:     cfg.Generate.EnableClientJsonOmitzeroTag,
	}

	if err := cfg.Client.Check(); err != nil {
		return nil, fmt.Errorf("config.exec: %w", err)
	}

	return &cfg, nil
}

// LoadSchema load and parses the schema from a local file or a remote server
func (c *Config) LoadSchema(ctx context.Context) error {
	var schema *ast.Schema
	if c.SchemaFilename != nil {
		s, err := c.loadLocalSchema()
		if err != nil {
			return fmt.Errorf("load local schema failed: %w", err)
		}

		schema = s
	} else {
		s, err := c.loadRemoteSchema(ctx)
		if err != nil {
			return fmt.Errorf("load remote schema failed: %w", err)
		}

		schema = s
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
	if err := gqlclient.Post(ctx, "Query", introspection.Introspection, &res, nil); err != nil {
		return nil, fmt.Errorf("introspection query failed: %w", err)
	}

	schema, err := validator.ValidateSchemaDocument(introspection.ParseIntrospectionQuery(c.Endpoint.URL, res))
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	return schema, nil
}

func (c *Config) loadLocalSchema() (*ast.Schema, error) {
	schema, err := gqlparser.LoadSchema(c.GQLConfig.Sources...)
	if err != nil {
		return nil, fmt.Errorf("loadLocalSchema: %w", err)
	}

	return schema, nil
}
