package skip_include_directives_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/skip_include_directives/expected"
)

// newClient returns a generated client whose server always answers with response.
func newClient(t *testing.T, response string) *generated.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	return generated.NewClient(http.DefaultClient, server.URL, &clientv2.Options{})
}

func TestDecode_everyFieldIncluded(t *testing.T) {
	t.Parallel()

	client := newClient(t, `{"data":{"item":{"id":"item-1","name":"first","status":"ACTIVE","parent":{"id":"item-0"},"children":[{"id":"item-2"}],"itemName":"first","renamed":"first"}}}`)

	res, err := client.ItemWithDirectives(context.Background(), "item-1", true, false)
	require.NoError(t, err)

	item := res.GetItem()
	require.Equal(t, "item-1", item.GetID())
	require.Equal(t, "first", item.GetName())
	require.Equal(t, generated.StatusActive, *item.GetStatus())
	require.Equal(t, "item-0", item.GetParent().GetID())
	require.Len(t, item.GetChildren(), 1)
	require.Equal(t, "first", item.GetItemName())
	require.Equal(t, "first", item.GetItem().GetRenamed())
}

// A server leaves a skipped field out of the response, also when the field is non-null.
func TestDecode_everyDirectedFieldLeftOut(t *testing.T) {
	t.Parallel()

	client := newClient(t, `{"data":{"item":{"id":"item-1"}}}`)

	res, err := client.ItemWithDirectives(context.Background(), "item-1", false, true)
	require.NoError(t, err)

	item := res.GetItem()
	require.Equal(t, "item-1", item.GetID())
	require.Empty(t, item.GetName())
	require.Empty(t, *item.GetStatus())
	require.Nil(t, item.GetParent())
	require.Empty(t, item.GetChildren())
	require.Empty(t, item.GetItemName())
	require.Empty(t, item.GetItem().GetRenamed())
}
