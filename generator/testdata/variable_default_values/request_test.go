package variable_default_values_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/variable_default_values/expected"
)

// newClient returns a generated client whose server always answers with response,
// and the variables of the last request the client sent. A request without
// variables is recorded as an empty object.
func newClient(t *testing.T, response string) (*generated.Client, *json.RawMessage) {
	t.Helper()

	var sentVariables json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var request struct {
			Variables json.RawMessage `json:"variables"`
		}
		require.NoError(t, json.Unmarshal(body, &request))

		sentVariables = request.Variables
		if sentVariables == nil {
			sentVariables = json.RawMessage(`{}`)
		}

		_, err = w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	return generated.NewClient(http.DefaultClient, server.URL, &clientv2.Options{}), &sentVariables
}

func TestRequest_nilVariableWithDefaultIsLeftOut(t *testing.T) {
	t.Parallel()

	client, sentVariables := newClient(t, `{"data":{"items":[],"more":[]}}`)

	_, err := client.ItemsWithDefaults(context.Background(), nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(*sentVariables))
}

func TestRequest_setVariableWithDefaultIsSent(t *testing.T) {
	t.Parallel()

	client, sentVariables := newClient(t, `{"data":{"items":[],"more":[]}}`)

	first := 20
	requiredFirst := 30
	status := generated.StatusArchived
	name := "b"

	_, err := client.ItemsWithDefaults(context.Background(), &first, &requiredFirst, &status, &generated.ItemFilter{Name: &name}, []string{})
	require.NoError(t, err)
	require.JSONEq(t, `{"first":20,"requiredFirst":30,"status":"ARCHIVED","filter":{"name":"b"},"tags":[]}`, string(*sentVariables))
}

func TestRequest_nilVariableWithoutDefaultIsSentAsNull(t *testing.T) {
	t.Parallel()

	client, sentVariables := newClient(t, `{"data":{"items":[]}}`)

	_, err := client.ItemsWithAndWithoutDefault(context.Background(), nil, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"status":null}`, string(*sentVariables))
}
