package nested_fragment_spread_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/nested_fragment_spread/expected"
)

func serverURL(t *testing.T) string {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return v
}

// TestE2E_GetPersonBasic tests that the generated client can unmarshal
// a response for a query using nested fragment spreads (issue #291).
// After fix: PersonBasic has flat fields {Address, Name} merged from NameFrag.
func TestE2E_GetPersonBasic(t *testing.T) {
	client := generated.NewClient(http.DefaultClient, serverURL(t), &clientv2.Options{})

	res, err := client.GetPersonBasic(context.Background())
	if err != nil {
		t.Fatalf("GetPersonBasic failed: %v", err)
	}

	person := res.GetPerson()
	if person == nil {
		t.Fatal("person is nil")
	}

	t.Logf("Address: %q", person.GetAddress())
	t.Logf("Name: %q", person.GetName())

	if person.GetName() == "" {
		t.Error("expected non-empty name (merged from NameFrag)")
	}

	if person.GetAddress() == "" {
		t.Error("expected non-empty address")
	}
}

// TestE2E_GetFullProfile tests nested object field merge case.
// After fix: FullProfile has merged Profile {Avatar, Bio} without ProfileBioFrag pointer.
func TestE2E_GetFullProfile(t *testing.T) {
	client := generated.NewClient(http.DefaultClient, serverURL(t), &clientv2.Options{})

	res, err := client.GetFullProfile(context.Background())
	if err != nil {
		t.Fatalf("GetFullProfile failed: %v", err)
	}

	person := res.GetPerson()
	if person == nil {
		t.Fatal("person is nil")
	}

	profile := person.GetProfile()
	t.Logf("Profile Bio: %q, Avatar: %q", profile.GetBio(), profile.GetAvatar())

	if profile.GetBio() == "" {
		t.Error("expected non-empty bio from merged profile")
	}

	if profile.GetAvatar() == "" {
		t.Error("expected non-empty avatar from merged profile")
	}
}

// TestE2E_GetFragA tests multi-level nesting (FragA -> FragB -> FragC).
// After fix: FragA has flat fields {Address, Age} plus FragC *FragC from FragB's spread.
func TestE2E_GetFragA(t *testing.T) {
	client := generated.NewClient(http.DefaultClient, serverURL(t), &clientv2.Options{})

	res, err := client.GetFragA(context.Background())
	if err != nil {
		t.Fatalf("GetFragA failed: %v", err)
	}

	person := res.GetPerson()
	if person == nil {
		t.Fatal("person is nil")
	}

	t.Logf("Age: %d", person.GetAge())
	t.Logf("Address: %q", person.GetAddress())

	if person.GetAddress() == "" {
		t.Error("expected non-empty address (merged from FragB)")
	}

	t.Logf("Name: %q", person.GetName())

	if person.GetName() == "" {
		t.Error("expected non-empty name (merged from FragC via FragB)")
	}
}
