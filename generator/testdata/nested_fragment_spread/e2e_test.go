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

// TestE2E_GetPersonBasic tests simple nested fragment spread with conversion getter.
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

	if person.GetName() == "" {
		t.Error("expected non-empty name")
	}

	if person.GetAddress() == "" {
		t.Error("expected non-empty address")
	}

	// Conversion getter: PersonBasic -> NameFrag
	nameFrag := person.GetNameFrag()
	if nameFrag.GetName() != person.GetName() {
		t.Errorf("GetNameFrag().Name = %q, want %q", nameFrag.GetName(), person.GetName())
	}
}

// TestE2E_GetFullProfile tests nested object field merge with conversion getter.
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

	if person.GetProfile().GetBio() == "" {
		t.Error("expected non-empty bio")
	}

	if person.GetProfile().GetAvatar() == "" {
		t.Error("expected non-empty avatar")
	}

	// Conversion getter: FullProfile -> ProfileBioFrag (different nested struct type)
	profileBioFrag := person.GetProfileBioFrag()
	if profileBioFrag.GetProfile().GetBio() != person.GetProfile().GetBio() {
		t.Errorf("GetProfileBioFrag().Profile.Bio = %q, want %q",
			profileBioFrag.GetProfile().GetBio(), person.GetProfile().GetBio())
	}
}

// TestE2E_GetFragA tests multi-level nesting with conversion getter chain.
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

	if person.GetName() == "" {
		t.Error("expected non-empty name")
	}

	if person.GetAddress() == "" {
		t.Error("expected non-empty address")
	}

	// Conversion getter chain: FragA -> FragB -> FragC
	fragB := person.GetFragB()
	if fragB.GetAddress() != person.GetAddress() {
		t.Errorf("GetFragB().Address = %q, want %q", fragB.GetAddress(), person.GetAddress())
	}

	if fragB.GetName() != person.GetName() {
		t.Errorf("GetFragB().Name = %q, want %q", fragB.GetName(), person.GetName())
	}

	fragC := fragB.GetFragC()
	if fragC.GetName() != person.GetName() {
		t.Errorf("GetFragB().GetFragC().Name = %q, want %q", fragC.GetName(), person.GetName())
	}
}
