package nullable_input_omittable_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/nullable_input_omittable/expected"
)

func newClient(t *testing.T) *generated.Client {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return generated.NewClient(http.DefaultClient, v, &clientv2.Options{})
}

// TestE2E_ListUsers_WithNilFilter tests that passing nil filter works correctly.
func TestE2E_ListUsers_WithNilFilter(t *testing.T) {
	client := newClient(t)

	res, err := client.ListUsers(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	users := res.GetUsers()
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}

	for i, user := range users {
		if user.GetID() != "user-1" {
			t.Errorf("users[%d].ID = %q, want %q", i, user.GetID(), "user-1")
		}

		if user.GetName() != "Alice" {
			t.Errorf("users[%d].Name = %q, want %q", i, user.GetName(), "Alice")
		}

		if user.GetAge() == nil || *user.GetAge() != 25 {
			t.Errorf("users[%d].Age = %v, want 25", i, user.GetAge())
		}
	}
}

// TestE2E_ListUsers_WithOmittableFilter tests that Omittable input fields
// correctly serialize when values are set.
func TestE2E_ListUsers_WithOmittableFilter(t *testing.T) {
	client := newClient(t)

	name := "Alice"
	filter := &generated.UserFilter{
		Name: graphql.OmittableOf(&name),
	}

	res, err := client.ListUsers(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	users := res.GetUsers()
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}

	for i, user := range users {
		if user.GetID() != "user-1" {
			t.Errorf("users[%d].ID = %q, want %q", i, user.GetID(), "user-1")
		}

		if user.GetName() != "Alice" {
			t.Errorf("users[%d].Name = %q, want %q", i, user.GetName(), "Alice")
		}
	}
}

// TestE2E_ListUsers_WithEmptyOmittableFilter tests that Omittable fields
// that are not set are correctly omitted from the request.
func TestE2E_ListUsers_WithEmptyOmittableFilter(t *testing.T) {
	client := newClient(t)

	// All fields are unset Omittable — they should be omitted from serialization
	filter := &generated.UserFilter{}

	res, err := client.ListUsers(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	users := res.GetUsers()
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}
}
