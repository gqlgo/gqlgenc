package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/99designs/gqlgen/codegen/config"

	"github.com/gqlgo/gqlgenc/introspection"

	"github.com/vektah/gqlparser/v2"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()
	t.Run("config does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("doesnotexist.yml")
		require.Error(t, err)
	})

	t.Run("malformed config", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("testdata/cfg/malformedconfig.yml")
		require.EqualError(t, err, "unable to parse config: [1:1] string was used where mapping is expected\n>  1 | asdf\n       ^\n")
	})

	t.Run("'schema' and 'endpoint' both specified", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("testdata/cfg/schema_endpoint.yml")
		require.EqualError(t, err, "'schema' and 'endpoint' both specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	})

	t.Run("neither 'schema' nor 'endpoint' specified", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("testdata/cfg/no_source.yml")
		require.EqualError(t, err, "neither 'schema' nor 'endpoint' specified. Use schema to load from a local file, use endpoint to load from a remote server (using introspection)")
	})

	t.Run("unknown keys", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("testdata/cfg/unknownkeys.yml")
		require.EqualError(t, err, "unable to parse config: [3:1] unknown field \"unknown\"\n   1 | schema:\n   2 |   - outer\n>  3 | unknown: foo\n       ^\n")
	})

	t.Run("globbed filenames", func(t *testing.T) {
		t.Parallel()

		loadConfig, err := LoadConfig("testdata/cfg/glob.yml")
		require.NoError(t, err)

		if runtime.GOOS == "windows" {
			require.Equal(t, loadConfig.SchemaFilename[0], `testdata\cfg\glob\bar\bar with spaces.graphql`)
			require.Equal(t, loadConfig.SchemaFilename[1], `testdata\cfg\glob\foo\foo.graphql`)
		} else {
			require.Equal(t, loadConfig.SchemaFilename[0], "testdata/cfg/glob/bar/bar with spaces.graphql")
			require.Equal(t, loadConfig.SchemaFilename[1], "testdata/cfg/glob/foo/foo.graphql")
		}
	})

	t.Run("unwalkable path", func(t *testing.T) {
		t.Parallel()

		_, err := LoadConfig("testdata/cfg/unwalkable.yml")
		if runtime.GOOS == "windows" {
			require.EqualError(t, err, "failed to walk schema at root not_walkable/: CreateFile not_walkable/: The system cannot find the file specified.")
		} else {
			require.EqualError(t, err, "failed to walk schema at root not_walkable/: lstat not_walkable/: no such file or directory")
		}
	})

	t.Run("generate", func(t *testing.T) {
		t.Parallel()

		loadConfig, err := LoadConfig("testdata/cfg/generate.yml")
		require.NoError(t, err)
		require.Equal(t, true, loadConfig.Generate.ShouldGenerateClient())
		require.Equal(t, loadConfig.Generate.UnamedPattern, "Empty")
		require.Equal(t, loadConfig.Generate.Suffix.Mutation, "Bar")
		require.Equal(t, loadConfig.Generate.Suffix.Query, "Foo")
		require.Equal(t, loadConfig.Generate.Prefix.Mutation, "Hoge")
		require.Equal(t, loadConfig.Generate.Prefix.Query, "Data")
	})

	t.Run("generate skip client", func(t *testing.T) {
		t.Parallel()

		c, err := LoadConfig("testdata/cfg/generate_client_false.yml")
		require.NoError(t, err)

		require.Equal(t, false, c.Generate.ShouldGenerateClient())
	})

	t.Run("nullable input omittable", func(t *testing.T) {
		t.Parallel()

		c, err := LoadConfig("testdata/cfg/nullable_input_omittable.yml")
		require.NoError(t, err)

		require.True(t, c.GQLConfig.NullableInputOmittable)
	})
	t.Run("omitempty, omitzero", func(t *testing.T) {
		t.Parallel()

		c, err := LoadConfig("testdata/cfg/omitempty_omitzero.yml")
		require.NoError(t, err)

		require.True(t, *c.GQLConfig.EnableModelJsonOmitemptyTag)
		require.True(t, *c.GQLConfig.EnableModelJsonOmitzeroTag)
	})

	t.Run("model omit tags split", func(t *testing.T) {
		t.Parallel()

		c, err := LoadConfig("testdata/cfg/model_omit_tags_split.yml")
		require.NoError(t, err)

		require.True(t, *c.Generate.EnableClientJsonOmitemptyTag)
		require.False(t, *c.Generate.EnableModelJsonOmitemptyTag)
		require.False(t, *c.Generate.EnableClientJsonOmitzeroTag)
		require.True(t, *c.Generate.EnableModelJsonOmitzeroTag)
		require.False(t, *c.GQLConfig.EnableModelJsonOmitemptyTag)
		require.True(t, *c.GQLConfig.EnableModelJsonOmitzeroTag)
	})
}

func TestLoadConfig_LoadSchema(t *testing.T) {
	t.Parallel()

	t.Run("correct schema", func(t *testing.T) {
		t.Parallel()

		mockServer, closeServer := newMockRemoteServer(t, responseFromFile("testdata/remote/response_ok.json"))
		defer closeServer()

		config := &Config{
			GQLConfig: &config.Config{},
			Endpoint: &EndPointConfig{
				URL: mockServer.URL,
			},
		}

		err := config.LoadSchema(context.Background())
		require.NoError(t, err)
	})

	t.Run("invalid schema", func(t *testing.T) {
		t.Parallel()

		mockServer, closeServer := newMockRemoteServer(t, responseFromFile("testdata/remote/response_invalid_schema.json"))
		defer closeServer()

		config := &Config{
			GQLConfig: &config.Config{},
			Endpoint: &EndPointConfig{
				URL: mockServer.URL,
			},
		}

		err := config.LoadSchema(context.Background())
		require.Equal(t, fmt.Sprintf("load remote schema failed: validation error: %s:0:0: OBJECT Query: must define one or more fields.", mockServer.URL), err.Error())
	})

	t.Run("neither schema nor endpoint", func(t *testing.T) {
		t.Parallel()

		config := &Config{GQLConfig: &config.Config{}}

		err := config.LoadSchema(context.Background())
		require.ErrorContains(t, err, "neither 'schema' nor 'endpoint' specified")
	})
}

func TestConfig_schemaSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		schemaFilename StringList
		endpoint       *EndPointConfig
		want           schemaSource
		wantCheckErr   string
	}{
		{name: "schema only", schemaFilename: StringList{"schema.graphql"}, want: schemaSourceLocal},
		{name: "endpoint only", endpoint: &EndPointConfig{URL: "http://localhost"}, want: schemaSourceRemote},
		{name: "both", schemaFilename: StringList{"schema.graphql"}, endpoint: &EndPointConfig{URL: "http://localhost"}, want: schemaSourceAmbiguous, wantCheckErr: "'schema' and 'endpoint' both specified"},
		{name: "neither", want: schemaSourceMissing, wantCheckErr: "neither 'schema' nor 'endpoint' specified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &Config{SchemaFilename: tt.schemaFilename, Endpoint: tt.endpoint}

			require.Equal(t, tt.want, config.schemaSource())

			err := config.checkSchemaSource()
			if tt.wantCheckErr == "" {
				require.NoError(t, err)

				return
			}

			require.ErrorContains(t, err, tt.wantCheckErr)
		})
	}
}

func TestLoadConfig_LoadSchema_capabilities(t *testing.T) {
	t.Parallel()

	const repeatedDirectiveQuery = `query Test { test(old: true) @tag(name: "a") @tag(name: "b") }`

	t.Run("a server with every capability reports deprecated inputs and repeatable directives", func(t *testing.T) {
		t.Parallel()

		capabilityServer := newCapabilityServer(t, responseFromFile("testdata/remote/response_capabilities_all.json"))

		cfg := &Config{
			GQLConfig: &config.Config{},
			Endpoint:  &EndPointConfig{URL: capabilityServer.URL},
		}

		require.NoError(t, cfg.LoadSchema(context.Background()))

		allCapabilities := introspection.Capabilities{
			FieldArgumentDeprecation:     true,
			InputFieldDeprecation:        true,
			DirectiveArgumentDeprecation: true,
			InputValueDeprecation:        true,
			RepeatableDirective:          true,
		}
		require.Equal(t, []string{introspection.CapabilityQueryDocument, introspection.NewQuery(allCapabilities)}, capabilityServer.receivedQueries())

		schema := cfg.GQLConfig.Schema

		deprecatedArgument := schema.Types["Query"].Fields.ForName("test").Arguments.ForName("old")
		require.NotNil(t, deprecatedArgument)
		require.Equal(t, "use new", deprecatedArgument.Directives.ForName("deprecated").Arguments.ForName("reason").Value.Raw)
		require.Nil(t, schema.Types["Query"].Fields.ForName("test").Arguments.ForName("new").Directives.ForName("deprecated"))

		deprecatedInputField := schema.Types["Filter"].Fields.ForName("old")
		require.NotNil(t, deprecatedInputField)
		require.Equal(t, "use new", deprecatedInputField.Directives.ForName("deprecated").Arguments.ForName("reason").Value.Raw)
		require.Nil(t, schema.Types["Filter"].Fields.ForName("new").Directives.ForName("deprecated"))

		require.True(t, schema.Directives["tag"].IsRepeatable)

		_, errs := gqlparser.LoadQuery(schema, repeatedDirectiveQuery)
		require.Empty(t, errs)
	})

	fallbacks := []struct {
		name               string
		capabilityResponse any
	}{
		{"a server without the capabilities", responseFromFile("testdata/remote/response_capabilities_none.json")},
		{"a server that rejects the capability query", json.RawMessage(`{"errors":[{"message":"Cannot query field \"__type\" on type \"Query\"."}]}`)},
	}

	for _, fallback := range fallbacks {
		t.Run(fallback.name+" gets the query without optional parts", func(t *testing.T) {
			t.Parallel()

			capabilityServer := newCapabilityServer(t, fallback.capabilityResponse)

			cfg := &Config{
				GQLConfig: &config.Config{},
				Endpoint:  &EndPointConfig{URL: capabilityServer.URL},
			}

			require.NoError(t, cfg.LoadSchema(context.Background()))
			require.Equal(t, []string{introspection.CapabilityQueryDocument, introspection.Introspection}, capabilityServer.receivedQueries())
		})
	}
}

// capabilityServer is a remote server that answers the capability query and
// the introspection query with different responses.
type capabilityServer struct {
	*httptest.Server

	mutex   sync.Mutex
	queries []string
}

// newCapabilityServer starts a server that answers the capability query with
// capabilityResponse and any other query with the schema in
// testdata/remote/response_deprecated_inputs.json. The server is closed when
// the test ends.
func newCapabilityServer(t *testing.T, capabilityResponse any) *capabilityServer {
	t.Helper()

	server := &capabilityServer{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		var request struct {
			OperationName string `json:"operationName"`
			Query         string `json:"query"`
		}

		if !assert.NoError(t, json.NewDecoder(req.Body).Decode(&request)) {
			return
		}

		server.mutex.Lock()
		server.queries = append(server.queries, request.Query)
		server.mutex.Unlock()

		response := any(responseFromFile("testdata/remote/response_deprecated_inputs.json"))
		if request.OperationName == "Capabilities" {
			response = capabilityResponse
		}

		var responseBody []byte

		switch v := response.(type) {
		case json.RawMessage:
			responseBody = v
		case responseFromFile:
			responseBody = v.load(t)
		}

		_, err := writer.Write(responseBody)
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	return server
}

// receivedQueries returns the queries the server has received, in order.
func (s *capabilityServer) receivedQueries() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return slices.Clone(s.queries)
}

type mockRemoteServer struct {
	*httptest.Server

	body []byte
}

func newMockRemoteServer(t *testing.T, response any) (mock *mockRemoteServer, closeServer func()) {
	t.Helper()

	mock = &mockRemoteServer{
		Server: httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			var err error

			mock.body, err = io.ReadAll(req.Body)
			require.NoError(t, err)

			var responseBody []byte

			switch v := response.(type) {
			case json.RawMessage:
				responseBody = v
			case responseFromFile:
				responseBody = v.load(t)
			default:
				responseBody, err = json.Marshal(response)
				require.NoError(t, err)
			}

			_, err = writer.Write(responseBody)
			require.NoError(t, err)
		})),
	}

	return mock, func() { mock.Close() }
}

type responseFromFile string

func (f responseFromFile) load(t *testing.T) []byte {
	t.Helper()

	content, err := os.ReadFile(string(f))
	require.NoError(t, err)

	return content
}
