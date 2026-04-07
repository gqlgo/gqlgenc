package nested_fragment_spread_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	"github.com/gqlgo/gqlgenc/generator/testdata/nested_fragment_spread/expected"
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
func TestE2E_GetPersonBasic(t *testing.T) {
	t.Skip("issue #291: nested fragment spread unmarshal fails with named pointer field")
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

	nameFrag := person.GetNameFrag()
	if nameFrag == nil {
		t.Fatal("NameFrag is nil — nested fragment spread not populated")
	}

	t.Logf("Name (via GetNameFrag): %q", nameFrag.GetName())

	if nameFrag.GetName() == "" {
		t.Error("expected non-empty name from NameFrag")
	}
}

// TestE2E_GetFullProfile tests nested object field merge case.
func TestE2E_GetFullProfile(t *testing.T) {
	t.Skip("issue #291: nested fragment spread unmarshal fails with named pointer field")
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

	profileBioFrag := person.GetProfileBioFrag()
	if profileBioFrag == nil {
		t.Fatal("ProfileBioFrag is nil — nested fragment spread not populated")
	}

	t.Logf("Bio (via GetProfileBioFrag): %q", profileBioFrag.GetProfile().GetBio())
}

// TestE2E_GetFragA tests multi-level nesting (FragA -> FragB -> FragC).
func TestE2E_GetFragA(t *testing.T) {
	t.Skip("issue #291: nested fragment spread unmarshal fails with named pointer field")
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

	fragB := person.GetFragB()
	if fragB == nil {
		t.Fatal("FragB is nil — nested fragment spread not populated")
	}

	t.Logf("Address (via GetFragB): %q", fragB.GetAddress())

	fragC := fragB.GetFragC()
	if fragC == nil {
		t.Fatal("FragC is nil — nested fragment spread chain not populated")
	}

	t.Logf("Name (via GetFragB().GetFragC()): %q", fragC.GetName())

	if fragC.GetName() == "" {
		t.Error("expected non-empty name from FragC via chain")
	}
}
