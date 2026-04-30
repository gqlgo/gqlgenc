package nullable_boolean_response_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/nullable_boolean_response/expected"
)

func newClient(t *testing.T) *generated.Client {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return generated.NewClient(http.DefaultClient, v, &clientv2.Options{})
}

// TestE2E_GetThing_NullableBoolean verifies that nullable Boolean fields
// in response types are correctly generated as *bool (not bool).
func TestE2E_GetThing_NullableBoolean(t *testing.T) {
	client := newClient(t)

	res, err := client.GetThing(context.Background())
	if err != nil {
		t.Fatalf("GetThing failed: %v", err)
	}

	thing := res.GetGetThing()
	// flag is nullable — GetFlag() must return *bool, accepting nil
	var _ *bool = thing.GetFlag()
}
