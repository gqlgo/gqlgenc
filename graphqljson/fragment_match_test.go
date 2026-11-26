package graphqljson_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/graphqljson"
)

// The types generated for this query:
//
//	__typename
//	name
//	... on Image { id url }
//	... on Video { id duration }
//	... on Node  { id createdAt }    Image and Video implement Node
type (
	keyOrderImageFragment struct {
		ID  string `graphql:"id"`
		URL string `graphql:"url"`
	}

	keyOrderVideoFragment struct {
		ID       string `graphql:"id"`
		Duration string `graphql:"duration"`
	}

	keyOrderNodeFragment struct {
		ID        string `graphql:"id"`
		CreatedAt string `graphql:"createdAt"`
	}

	keyOrderPointerFragments struct {
		Typename string                 `graphql:"__typename"`
		Name     string                 `graphql:"name"`
		Image    *keyOrderImageFragment `graphql:"... on Image" gqlgenc:"possibleTypes=Image"`
		Video    *keyOrderVideoFragment `graphql:"... on Video" gqlgenc:"possibleTypes=Video"`
		Node     *keyOrderNodeFragment  `graphql:"... on Node" gqlgenc:"possibleTypes=Image,Video"`
	}

	keyOrderValueFragments struct {
		Typename string                `graphql:"__typename"`
		Name     string                `graphql:"name"`
		Image    keyOrderImageFragment `graphql:"... on Image" gqlgenc:"possibleTypes=Image"`
		Video    keyOrderVideoFragment `graphql:"... on Video" gqlgenc:"possibleTypes=Video"`
		Node     keyOrderNodeFragment  `graphql:"... on Node" gqlgenc:"possibleTypes=Image,Video"`
	}
)

// jsonMember is one "key": value member of a JSON object.
type jsonMember string

// jsonMembers are the members of a JSON object, in the order they are sent.
type jsonMembers []jsonMember

// object returns the JSON object holding the members in their order.
func (m jsonMembers) object() string {
	members := make([]string, 0, len(m))
	for _, member := range m {
		members = append(members, string(member))
	}

	return "{" + strings.Join(members, ",") + "}"
}

// permutations returns every order of the members.
func (m jsonMembers) permutations() []jsonMembers {
	if len(m) <= 1 {
		return []jsonMembers{append(jsonMembers(nil), m...)}
	}

	var result []jsonMembers

	for i, first := range m {
		rest := make(jsonMembers, 0, len(m)-1)
		rest = append(rest, m[:i]...)
		rest = append(rest, m[i+1:]...)

		for _, permutation := range rest.permutations() {
			result = append(result, append(jsonMembers{first}, permutation...))
		}
	}

	return result
}

// TestUnmarshalData_fragmentsAreIndependentOfKeyOrder checks, for every order of
// the keys, that each fragment that applies to __typename has all of its fields,
// and that every other fragment is empty: nil for a pointer.
func TestUnmarshalData_fragmentsAreIndependentOfKeyOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		members     jsonMembers
		wantPointer keyOrderPointerFragments
		wantValue   keyOrderValueFragments
	}{
		{
			name:    "Image",
			members: jsonMembers{`"__typename":"Image"`, `"name":"n"`, `"id":"1"`, `"url":"u"`, `"createdAt":"c"`},
			wantPointer: keyOrderPointerFragments{
				Typename: "Image",
				Name:     "n",
				Image:    &keyOrderImageFragment{ID: "1", URL: "u"},
				Node:     &keyOrderNodeFragment{ID: "1", CreatedAt: "c"},
			},
			wantValue: keyOrderValueFragments{
				Typename: "Image",
				Name:     "n",
				Image:    keyOrderImageFragment{ID: "1", URL: "u"},
				Node:     keyOrderNodeFragment{ID: "1", CreatedAt: "c"},
			},
		},
		{
			name:    "Video",
			members: jsonMembers{`"__typename":"Video"`, `"name":"n"`, `"id":"1"`, `"duration":"d"`, `"createdAt":"c"`},
			wantPointer: keyOrderPointerFragments{
				Typename: "Video",
				Name:     "n",
				Video:    &keyOrderVideoFragment{ID: "1", Duration: "d"},
				Node:     &keyOrderNodeFragment{ID: "1", CreatedAt: "c"},
			},
			wantValue: keyOrderValueFragments{
				Typename: "Video",
				Name:     "n",
				Video:    keyOrderVideoFragment{ID: "1", Duration: "d"},
				Node:     keyOrderNodeFragment{ID: "1", CreatedAt: "c"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			permutations := tt.members.permutations()
			require.Len(t, permutations, 120)

			for _, permutation := range permutations {
				object := permutation.object()

				var gotPointer keyOrderPointerFragments
				require.NoError(t, graphqljson.UnmarshalData([]byte(object), &gotPointer), object)
				require.Equal(t, tt.wantPointer, gotPointer, object)

				var gotValue keyOrderValueFragments
				require.NoError(t, graphqljson.UnmarshalData([]byte(object), &gotValue), object)
				require.Equal(t, tt.wantValue, gotValue, object)
			}
		})
	}
}
