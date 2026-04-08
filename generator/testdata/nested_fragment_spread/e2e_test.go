package nested_fragment_spread_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/gqlgo/gqlgenc/clientv2"
	generated "github.com/gqlgo/gqlgenc/generator/testdata/nested_fragment_spread/expected"
)

func newClient(t *testing.T) *generated.Client {
	t.Helper()

	v := os.Getenv("GQLDUMMY_URL")
	if v == "" {
		t.Skip("GQLDUMMY_URL not set — start gqldummy and set GQLDUMMY_URL to run this test")
	}

	return generated.NewClient(http.DefaultClient, v, &clientv2.Options{})
}

// TestE2E_GetPersonBasic tests simple nested fragment spread with conversion getter.
func TestE2E_GetPersonBasic(t *testing.T) {
	client := newClient(t)

	res, err := client.GetPersonBasic(context.Background())
	if err != nil {
		t.Fatalf("GetPersonBasic failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Homer Simpson" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Homer Simpson")
	}

	if person.GetAddress() != "742 Evergreen Terrace" {
		t.Errorf("Address = %q, want %q", person.GetAddress(), "742 Evergreen Terrace")
	}

	// Conversion getter: PersonBasic -> NameFrag
	nameFrag := person.GetNameFrag()
	if nameFrag.GetName() != "Homer Simpson" {
		t.Errorf("GetNameFrag().Name = %q, want %q", nameFrag.GetName(), "Homer Simpson")
	}
}

// TestE2E_GetFullProfile tests nested object field merge with conversion getter.
func TestE2E_GetFullProfile(t *testing.T) {
	client := newClient(t)

	res, err := client.GetFullProfile(context.Background())
	if err != nil {
		t.Fatalf("GetFullProfile failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetProfile().GetBio() != "Nuclear safety inspector" {
		t.Errorf("Profile.Bio = %q, want %q", person.GetProfile().GetBio(), "Nuclear safety inspector")
	}

	if person.GetProfile().GetAvatar() != "https://example.com/homer.png" {
		t.Errorf("Profile.Avatar = %q, want %q", person.GetProfile().GetAvatar(), "https://example.com/homer.png")
	}

	// Conversion getter: FullProfile -> ProfileBioFrag (recursive struct construction)
	profileBioFrag := person.GetProfileBioFrag()
	if profileBioFrag.GetProfile().GetBio() != "Nuclear safety inspector" {
		t.Errorf("GetProfileBioFrag().Profile.Bio = %q, want %q",
			profileBioFrag.GetProfile().GetBio(), "Nuclear safety inspector")
	}
}

// TestE2E_GetFragA tests multi-level nesting with conversion getter chain.
func TestE2E_GetFragA(t *testing.T) {
	client := newClient(t)

	res, err := client.GetFragA(context.Background())
	if err != nil {
		t.Fatalf("GetFragA failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Homer Simpson" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Homer Simpson")
	}

	if person.GetAddress() != "742 Evergreen Terrace" {
		t.Errorf("Address = %q, want %q", person.GetAddress(), "742 Evergreen Terrace")
	}

	if person.GetAge() != 39 {
		t.Errorf("Age = %d, want %d", person.GetAge(), 39)
	}

	// Chain: FragA -> FragB -> FragC
	fragB := person.GetFragB()
	if fragB.GetAddress() != "742 Evergreen Terrace" {
		t.Errorf("GetFragB().Address = %q, want %q", fragB.GetAddress(), "742 Evergreen Terrace")
	}

	if fragB.GetName() != "Homer Simpson" {
		t.Errorf("GetFragB().Name = %q, want %q", fragB.GetName(), "Homer Simpson")
	}

	fragC := fragB.GetFragC()
	if fragC.GetName() != "Homer Simpson" {
		t.Errorf("GetFragB().GetFragC().Name = %q, want %q", fragC.GetName(), "Homer Simpson")
	}
}

// TestE2E_GetMultiSpreadOverlap tests multiple fragment spreads with overlapping fields.
func TestE2E_GetMultiSpreadOverlap(t *testing.T) {
	client := newClient(t)

	res, err := client.GetMultiSpreadOverlap(context.Background())
	if err != nil {
		t.Fatalf("GetMultiSpreadOverlap failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Homer" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Homer")
	}

	if person.GetAge() != 39 {
		t.Errorf("Age = %d, want %d", person.GetAge(), 39)
	}

	if person.GetAddress() != "Springfield" {
		t.Errorf("Address = %q, want %q", person.GetAddress(), "Springfield")
	}

	// Conversion getters for both spread fragments
	ageFrag := person.GetAgeFrag()
	if ageFrag.GetAge() != 39 {
		t.Errorf("GetAgeFrag().Age = %d, want %d", ageFrag.GetAge(), 39)
	}

	nameAgeFrag := person.GetNameAgeFrag()
	if nameAgeFrag.GetName() != "Homer" {
		t.Errorf("GetNameAgeFrag().Name = %q, want %q", nameAgeFrag.GetName(), "Homer")
	}

	if nameAgeFrag.GetAge() != 39 {
		t.Errorf("GetNameAgeFrag().Age = %d, want %d", nameAgeFrag.GetAge(), 39)
	}
}

// TestE2E_GetDirectOverlap tests fragment spread + direct field overlap.
func TestE2E_GetDirectOverlap(t *testing.T) {
	client := newClient(t)

	res, err := client.GetDirectOverlap(context.Background())
	if err != nil {
		t.Fatalf("GetDirectOverlap failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Homer Simpson" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Homer Simpson")
	}

	if person.GetAge() != 10 {
		t.Errorf("Age = %d, want %d", person.GetAge(), 10)
	}

	nameFrag := person.GetNameFrag()
	if nameFrag.GetName() != "Homer Simpson" {
		t.Errorf("GetNameFrag().Name = %q, want %q", nameFrag.GetName(), "Homer Simpson")
	}
}

// TestE2E_GetPersonWithNickname tests fragment spread with nullable field.
func TestE2E_GetPersonWithNickname(t *testing.T) {
	client := newClient(t)

	res, err := client.GetPersonWithNickname(context.Background())
	if err != nil {
		t.Fatalf("GetPersonWithNickname failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Homer Simpson" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Homer Simpson")
	}

	if person.GetNickname() == nil || *person.GetNickname() != "Homie" {
		t.Errorf("Nickname = %v, want %q", person.GetNickname(), "Homie")
	}

	nicknameFrag := person.GetNicknameFrag()
	if nicknameFrag.GetNickname() == nil || *nicknameFrag.GetNickname() != "Homie" {
		t.Errorf("GetNicknameFrag().Nickname = %v, want %q", nicknameFrag.GetNickname(), "Homie")
	}
}

// TestE2E_GetPersonDirect tests non-fragment query coexists with fragment queries.
func TestE2E_GetPersonDirect(t *testing.T) {
	client := newClient(t)

	res, err := client.GetPersonDirect(context.Background())
	if err != nil {
		t.Fatalf("GetPersonDirect failed: %v", err)
	}

	person := res.GetPerson()
	if person.GetName() != "Marge Simpson" {
		t.Errorf("Name = %q, want %q", person.GetName(), "Marge Simpson")
	}

	if person.GetAddress() != "742 Evergreen Terrace" {
		t.Errorf("Address = %q, want %q", person.GetAddress(), "742 Evergreen Terrace")
	}

	if person.GetAge() != 36 {
		t.Errorf("Age = %d, want %d", person.GetAge(), 36)
	}
}

// TestE2E_GetPeople tests fragment spread in list query.
func TestE2E_GetPeople(t *testing.T) {
	client := newClient(t)

	res, err := client.GetPeople(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetPeople failed: %v", err)
	}

	if len(res.People) != 2 {
		t.Fatalf("len(people) = %d, want 2", len(res.People))
	}

	for i, person := range res.People {
		if person.GetName() != "Homer Simpson" {
			t.Errorf("people[%d].Name = %q, want %q", i, person.GetName(), "Homer Simpson")
		}

		if person.GetAddress() != "742 Evergreen Terrace" {
			t.Errorf("people[%d].Address = %q, want %q", i, person.GetAddress(), "742 Evergreen Terrace")
		}

		nameFrag := person.GetNameFrag()
		if nameFrag.GetName() != "Homer Simpson" {
			t.Errorf("people[%d].GetNameFrag().Name = %q, want %q", i, nameFrag.GetName(), "Homer Simpson")
		}
	}
}
