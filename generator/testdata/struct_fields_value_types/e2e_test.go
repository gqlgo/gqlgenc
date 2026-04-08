package struct_fields_value_types_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/struct_fields_value_types/expected"
)

func newClient(t *testing.T) *generated.Client {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return generated.NewClient(http.DefaultClient, v, &clientv2.Options{})
}

// TestE2E_GetUser_WithFragmentSpread tests that fragment spread fields
// are correctly deserialized when structFieldsAlwaysPointers is false.
func TestE2E_GetUser_WithFragmentSpread(t *testing.T) {
	client := newClient(t)

	res, err := client.GetUser(context.Background(), "any-id")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	user := res.GetUser()

	if user.GetID() != "user-789" {
		t.Errorf("ID = %q, want %q", user.GetID(), "user-789")
	}

	if user.GetName() != "Alice" {
		t.Errorf("Name = %q, want %q", user.GetName(), "Alice")
	}

	profile := user.GetProfile()
	if profile == nil {
		t.Fatal("Profile should not be nil")
	}

	if profile.GetBio() == nil || *profile.GetBio() != "Software engineer" {
		t.Errorf("Profile.Bio = %v, want %q", profile.GetBio(), "Software engineer")
	}
}

// TestE2E_GetUserDirect tests a non-fragment query with nested object
// when structFieldsAlwaysPointers is false.
func TestE2E_GetUserDirect(t *testing.T) {
	client := newClient(t)

	res, err := client.GetUserDirect(context.Background(), "any-id")
	if err != nil {
		t.Fatalf("GetUserDirect failed: %v", err)
	}

	user := res.GetUser()

	if user.GetID() != "user-direct" {
		t.Errorf("ID = %q, want %q", user.GetID(), "user-direct")
	}

	if user.GetName() != "Bob" {
		t.Errorf("Name = %q, want %q", user.GetName(), "Bob")
	}

	profile := user.GetProfile()
	if profile == nil {
		t.Fatal("Profile should not be nil")
	}

	if profile.GetBio() == nil || *profile.GetBio() != "Designer" {
		t.Errorf("Profile.Bio = %v, want %q", profile.GetBio(), "Designer")
	}

	if profile.GetAvatar() == nil || *profile.GetAvatar() != "https://example.com/bob.png" {
		t.Errorf("Profile.Avatar = %v, want %q", profile.GetAvatar(), "https://example.com/bob.png")
	}
}
