package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/goccy/go-yaml"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/plugin/federation"

	"github.com/vektah/gqlparser/v2/ast"
)

// and represents the config file.
type Config struct {
	GQLGencConfig *GQLGencConfig       `yaml:"gqlgenc"`
	GQLGenConfig  *gqlgenconfig.Config `yaml:"gqlgen"`
}

type GQLGencConfig struct {
	QueryGen        gqlgenconfig.PackageConfig `yaml:"querygen,omitempty"`
	ClientGen       gqlgenconfig.PackageConfig `yaml:"clientgen,omitempty"`
	Endpoint        *EndPointConfig            `yaml:"endpoint,omitempty"`
	Query           []string                   `yaml:"query"`
	ExportQueryType bool                       `yaml:"export_query_type,omitempty"`
}

// EndPointConfig are the allowed options for the 'endpoint' config.
type EndPointConfig struct {
	// TODO: テスト
	Headers http.Header `yaml:"headers,omitempty"`
	URL     string      `yaml:"url"`
	// TODO: 消す
	Client *http.Client `yaml:"-"`
}

func Init(ctx context.Context, configFileName string) (*Config, error) {
	c, err := loadConfig(configFileName)
	if err != nil {
		return nil, fmt.Errorf(": %w", err)
	}

	// Load schema
	// TODO: Add test for when SchemaFilename is not specified in config
	switch {
	case c.GQLGenConfig.SchemaFilename != nil:
		if err := c.GQLGenConfig.LoadSchema(); err != nil {
			return nil, fmt.Errorf("load local schema failed: %w", err)
		}
	case c.GQLGencConfig.Endpoint != nil:
		httpClient := c.GQLGencConfig.Endpoint.Client
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		schema, err := introspectionSchema(ctx, httpClient, c.GQLGencConfig.Endpoint.URL, c.GQLGencConfig.Endpoint.Headers)
		if err != nil {
			return nil, fmt.Errorf("introspect schema failed: %w", err)
		}
		c.GQLGenConfig.Schema = schema
	default:
		return nil, errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	// delete exist gen file
	if c.GQLGenConfig.Model.IsDefined() {
		// model gen file must be removed before cfg.PrepareSchema()
		_ = syscall.Unlink(c.GQLGenConfig.Model.Filename)
	}

	if c.GQLGencConfig.QueryGen.IsDefined() {
		_ = syscall.Unlink(c.GQLGencConfig.QueryGen.Filename)
	}

	if c.GQLGencConfig.ClientGen.IsDefined() {
		_ = syscall.Unlink(c.GQLGencConfig.ClientGen.Filename)
	}

	if err := c.GQLGenConfig.Init(); err != nil {
		return nil, fmt.Errorf("generating core failed: %w", err)
	}

	// sort Implements to ensure a deterministic output
	for _, implements := range c.GQLGenConfig.Schema.Implements {
		slices.SortFunc(implements, func(a, b *ast.Definition) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	return c, nil
}

// loadConfig loads and parses the config gqlgenc config.
func loadConfig(configFilename string) (*Config, error) {
	configContent, err := os.ReadFile(configFilename)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	var c Config

	yamlDecoder := yaml.NewDecoder(bytes.NewReader([]byte(os.ExpandEnv(string(configContent)))), yaml.DisallowUnknownField())
	if err := yamlDecoder.Decode(&c); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	// validation
	if c.GQLGenConfig.SchemaFilename != nil && c.GQLGencConfig.Endpoint != nil {
		return nil, errors.New("'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	if c.GQLGenConfig.SchemaFilename == nil && c.GQLGencConfig.Endpoint == nil {
		return nil, errors.New("neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	}

	if c.GQLGencConfig.ClientGen.IsDefined() && !c.GQLGencConfig.QueryGen.IsDefined() {
		return nil, errors.New("'clientgen' is set, 'querygen' must be set")
	}

	///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgen

	// check
	if err := c.GQLGenConfig.Model.Check(); err != nil {
		return nil, fmt.Errorf("model: %w", err)
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
	c.GQLGenConfig.Directives = make(map[string]gqlgenconfig.DirectiveConfig)
	c.GQLGenConfig.Exec = gqlgenconfig.ExecConfig{Filename: "generated.go"}
	c.GQLGenConfig.Resolver = gqlgenconfig.ResolverConfig{Filename: "generated.go"}
	c.GQLGenConfig.Federation = gqlgenconfig.PackageConfig{Filename: "generated.go"}

	///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// gqlgenc

	// validation
	if err := c.GQLGencConfig.QueryGen.Check(); err != nil {
		return nil, fmt.Errorf("querygen: %w", err)
	}

	if err := c.GQLGencConfig.ClientGen.Check(); err != nil {
		return nil, fmt.Errorf("clientgen: %w", err)
	}

	return &c, nil
}
