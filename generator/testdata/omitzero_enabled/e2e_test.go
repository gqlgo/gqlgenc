package omitzero_enabled_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/omitzero_enabled/expected"
)

func newClient(t *testing.T) *generated.Client {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return generated.NewClient(http.DefaultClient, v, &clientv2.Options{})
}

// TestE2E_GetUser_AllFieldsPresent tests that all fields (including nullable ones)
// are correctly deserialized with omitzero tags.
func TestE2E_GetUser_AllFieldsPresent(t *testing.T) {
	client := newClient(t)

	res, err := client.GetUser(context.Background(), "any-id")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	user := res.GetUser()

	if user.GetID() != "user-123" {
		t.Errorf("ID = %q, want %q", user.GetID(), "user-123")
	}

	if user.GetName() != "Alice" {
		t.Errorf("Name = %q, want %q", user.GetName(), "Alice")
	}

	if user.GetEmail() == nil || *user.GetEmail() != "alice@example.com" {
		t.Errorf("Email = %v, want %q", user.GetEmail(), "alice@example.com")
	}

	if user.GetAge() == nil || *user.GetAge() != 30 {
		t.Errorf("Age = %v, want 30", user.GetAge())
	}

	if user.GetBio() == nil || *user.GetBio() != "Hello world" {
		t.Errorf("Bio = %v, want %q", user.GetBio(), "Hello world")
	}
}

// TestE2E_GetUserNullable tests that nullable fields with omitzero tags
// correctly handle values from the server.
func TestE2E_GetUserNullable(t *testing.T) {
	client := newClient(t)

	res, err := client.GetUserNullable(context.Background(), "any-id")
	if err != nil {
		t.Fatalf("GetUserNullable failed: %v", err)
	}

	user := res.GetUser()

	if user.GetID() != "user-456" {
		t.Errorf("ID = %q, want %q", user.GetID(), "user-456")
	}

	if user.GetName() != "Bob" {
		t.Errorf("Name = %q, want %q", user.GetName(), "Bob")
	}

	// gqldummy returns non-null random values for these fields,
	// so we just verify the response was deserialized without error.
	// The key test is that json tags with omitzero work correctly.
	if user.GetEmail() == nil {
		t.Error("Email should not be nil (gqldummy returns a value)")
	}

	if user.GetAge() == nil {
		t.Error("Age should not be nil (gqldummy returns a value)")
	}

	if user.GetBio() == nil {
		t.Error("Bio should not be nil (gqldummy returns a value)")
	}
}
