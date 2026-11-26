package inline_fragment_same_type_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/inline_fragment_same_type/expected"
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

func TestDecode_repeatedInlineFragment(t *testing.T) {
	t.Parallel()

	client := newClient(t, `{"data":{"node":{"__typename":"Item","id":"item-1","name":"first","parent":{"id":"item-0"}}}}`)

	res, err := client.NodeWithRepeatedInlineFragment(context.Background(), "item-1")
	require.NoError(t, err)

	item := res.GetNode().GetItem()
	require.Equal(t, "item-1", item.GetID())
	require.Equal(t, "first", item.GetName())
	require.Equal(t, "item-0", item.GetParent().GetID())
	require.Empty(t, res.GetNode().GetComment().GetBody())
}

func TestDecode_untypedInlineFragment(t *testing.T) {
	t.Parallel()

	client := newClient(t, `{"data":{"node":{"id":"item-1","__typename":"Item"},"item":{"name":"first"}}}`)

	res, err := client.NodeWithUntypedInlineFragment(context.Background(), "item-1", true)
	require.NoError(t, err)

	require.Equal(t, "item-1", res.GetNode().GetID())
	require.Equal(t, "Item", *res.GetNode().GetTypename())
	require.Equal(t, "first", res.GetItem().GetName())
}
