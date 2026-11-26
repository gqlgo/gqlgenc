package graphqljson_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gqlgo/gqlgenc/graphqljson"
)

// The types generated with inlineFragmentAlwaysPointers for inline fragments
// written inside another inline fragment:
//
//	__typename
//	... on File { ... on Image { width } }
//	... on Node {
//	    id
//	    ... on Image { width }
//	    ... on Video { duration }
//	    ... on Media { ... on Image { width } }
//	}
type (
	nestedImageFragment struct {
		Width int `graphql:"width"`
	}

	nestedVideoFragment struct {
		Duration int `graphql:"duration"`
	}

	// nestedFileFragment has no field of its own: only the fragment inside it.
	nestedFileFragment struct {
		Image *nestedImageFragment `graphql:"... on Image" gqlgenc:"possibleTypes=Image"`
	}

	nestedMediaFragment struct {
		Image *nestedImageFragment `graphql:"... on Image" gqlgenc:"possibleTypes=Image"`
	}

	nestedNodeFragment struct {
		ID    string               `graphql:"id"`
		Image *nestedImageFragment `graphql:"... on Image" gqlgenc:"possibleTypes=Image"`
		Video *nestedVideoFragment `graphql:"... on Video" gqlgenc:"possibleTypes=Video"`
		Media *nestedMediaFragment `graphql:"... on Media" gqlgenc:"possibleTypes=Image,Video"`
	}

	nestedPointerFragments struct {
		Typename string              `graphql:"__typename"`
		File     *nestedFileFragment `graphql:"... on File" gqlgenc:"possibleTypes=Document,Image,Video"`
		Node     *nestedNodeFragment `graphql:"... on Node" gqlgenc:"possibleTypes=Document,Image,Video"`
	}
)

// TestUnmarshalData_nestedPointerFragments checks that an inline fragment
// inside a pointer inline fragment receives its fields, for every key order.
func TestUnmarshalData_nestedPointerFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		members jsonMembers
		want    nestedPointerFragments
	}{
		{
			name:    "the inner fragments apply, three levels deep",
			members: jsonMembers{`"__typename":"Image"`, `"id":"1"`, `"width":10`},
			want: nestedPointerFragments{
				Typename: "Image",
				File:     &nestedFileFragment{Image: &nestedImageFragment{Width: 10}},
				Node: &nestedNodeFragment{
					ID:    "1",
					Image: &nestedImageFragment{Width: 10},
					Media: &nestedMediaFragment{Image: &nestedImageFragment{Width: 10}},
				},
			},
		},
		{
			name:    "a field only an inner fragment has, without a field of the outer one",
			members: jsonMembers{`"__typename":"Image"`, `"width":10`},
			want: nestedPointerFragments{
				Typename: "Image",
				File:     &nestedFileFragment{Image: &nestedImageFragment{Width: 10}},
				Node: &nestedNodeFragment{
					Image: &nestedImageFragment{Width: 10},
					Media: &nestedMediaFragment{Image: &nestedImageFragment{Width: 10}},
				},
			},
		},
		{
			name:    "another inner fragment applies",
			members: jsonMembers{`"__typename":"Video"`, `"id":"1"`, `"duration":3`},
			want: nestedPointerFragments{
				Typename: "Video",
				Node: &nestedNodeFragment{
					ID:    "1",
					Video: &nestedVideoFragment{Duration: 3},
				},
			},
		},
		{
			name:    "no inner fragment applies",
			members: jsonMembers{`"__typename":"Document"`, `"id":"1"`},
			want: nestedPointerFragments{
				Typename: "Document",
				Node:     &nestedNodeFragment{ID: "1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, permutation := range tt.members.permutations() {
				object := permutation.object()

				var got nestedPointerFragments
				require.NoError(t, graphqljson.UnmarshalData([]byte(object), &got), object)
				require.Equal(t, tt.want, got, object)
			}
		})
	}
}
