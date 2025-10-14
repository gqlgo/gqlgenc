package graphqljson_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"encoding/json/jsontext"
	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/graphqljson"
)

func TestUnmarshalGraphQL_jsonTag(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo string `json:"baz"`
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "baz": "bar"
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{Foo: "bar"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_array(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo []string
		Bar []string
		Baz []string
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "foo": [
            "bar",
            "baz"
        ],
        "bar": [],
        "baz": null
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{
		Foo: []string{"bar", "baz"},
		Bar: []string{},
		Baz: []string(nil),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_arrayReset(t *testing.T) {
	t.Parallel()

	got := []string{"initial"}

	err := graphqljson.UnmarshalData([]byte(`["bar", "baz"]`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"bar", "baz"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_objectArray(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo []struct {
			Name string
		}
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "foo": [
            {"name": "bar"},
            {"name": "baz"}
        ]
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{
		Foo: []struct{ Name string }{
			{"bar"},
			{"baz"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_pointer(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo *string
		Bar *string
	}

	var got query
	got.Bar = new(string)

	err := graphqljson.UnmarshalData([]byte(`{
        "foo": "foo",
        "bar": null
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	foo := "foo"

	want := query{
		Foo: &foo,
		Bar: nil,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_objectPointerArray(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo []*struct {
			Name string
		}
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "foo": [
            {"name": "bar"},
            null,
            {"name": "baz"}
        ]
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{
		Foo: []*struct{ Name string }{
			{Name: "bar"},
			nil,
			{Name: "baz"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_unexportedField(t *testing.T) {
	t.Parallel()

	type query struct {
		//nolint:unused
		foo string
	}

	err := graphqljson.UnmarshalData([]byte(`{"foo": "bar"}`), new(query))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	got := err.Error()
	want := "decode graphql data: decode json: struct field for \"foo\" doesn't exist in any of 1 places to unmarshal (at byte offset 6)"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_multipleValues(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo string
	}

	err := graphqljson.UnmarshalData([]byte(`{"foo": "bar"}{"foo": "baz"}`), new(query))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got, want := err.Error(), "invalid token '{' after top-level value (at byte offset 15)"; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
}

func TestUnmarshalGraphQL_unknownJSONValue(t *testing.T) {
	t.Parallel()

	type query struct {
		Unknown jsontext.Value `json:",unknown"`
		Number  int            `json:"number"`
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "extra": {
            "foo": "bar"
        },
        "number": 1
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	var unknown map[string]string
	if err := json.Unmarshal(got.Unknown, &unknown); err != nil {
		t.Fatalf("parse unknown: %v", err)
	}
	if diff := cmp.Diff(unknown, map[string]string{"foo": "bar"}); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
	if got.Number != 1 {
		t.Errorf("unexpected number: %d", got.Number)
	}
}

func TestUnmarshalGraphQL_unknownJSONValue_map(t *testing.T) {
	t.Parallel()

	type query struct {
		Outputs jsontext.Value `json:"outputs,unknown"`
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "outputs":{
            "vpc":"1",
            "worker_role_arn":"2"
        }
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	var outputs map[string]string
	if err := json.Unmarshal(got.Outputs, &outputs); err != nil {
		t.Fatalf("parse unknown: %v", err)
	}
	if diff := cmp.Diff(outputs, map[string]string{"vpc": "1", "worker_role_arn": "2"}); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_multipleFragment(t *testing.T) {
	t.Parallel()

	type UserFragment1 struct {
		Name string `json:"name"`
	}

	type UserFragment2User struct {
		Name string `json:"name"`
	}

	type UserFragment2 struct {
		Name string            `json:"name"`
		User UserFragment2User `json:"user"`
	}

	type query struct {
		Typename string `json:"__typename"`
		Name     string `json:"name"`
		UserFragment1
		UserFragment2
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "__typename": "User",
        "name": "John Doe",
        "user": {
            "name": "Nested John"
        }
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{
		Typename:      "User",
		Name:          "John Doe",
		UserFragment1: UserFragment1{Name: "John Doe"},
		UserFragment2: UserFragment2{
			Name: "John Doe",
			User: UserFragment2User{Name: "Nested John"},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_unionStruct(t *testing.T) {
	t.Parallel()

	type issueTimelineItem struct {
		Typename    string `json:"__typename"`
		ClosedEvent *struct {
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
		}
		ReopenedEvent *struct {
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
		}
	}

	var got issueTimelineItem

	err := graphqljson.UnmarshalData([]byte(`{
        "__typename": "ClosedEvent",
        "actor": {
            "login": "shurcooL-test"
        }
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := issueTimelineItem{
		Typename: "ClosedEvent",
		ClosedEvent: &struct {
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
		}{
			Actor: struct {
				Login string `json:"login"`
			}{Login: "shurcooL-test"},
		},
		ReopenedEvent: nil,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_unionMissingTypename(t *testing.T) {
	t.Parallel()

	type issueTimelineItem struct {
		ClosedEvent   *struct{}
		ReopenedEvent *struct{}
	}

	var got issueTimelineItem

	err := graphqljson.UnmarshalData([]byte(`{
        "closedEvent": {},
        "reopenedEvent": {}
    }`), &got)
	if err == nil {
		t.Fatal("expected error for missing __typename")
	}
	if !strings.Contains(err.Error(), "__typename is required") {
		t.Errorf("expected '__typename is required' error, got: %v", err)
	}
}

func TestUnmarshalGraphQL_unionWithFragments(t *testing.T) {
	t.Parallel()

	type actor struct {
		Login string
		Name  string
	}

	type sharedFragment struct {
		Comment string
	}

	type timelineItem struct {
		Typename    string `json:"__typename"`
		ClosedEvent *struct {
			Actor actor `json:"actor"`
			Extra string
			sharedFragment
		}
		ReopenedEvent *struct {
			Actor actor `json:"actor"`
			Note  string
			sharedFragment
		}
	}

	payload := []byte(`{
        "__typename": "ReopenedEvent",
        "actor": {
        	"login": "user-b",
            "name": "User B"
        },
        "note": "re-opened",
        "comment": "shared field"
    }`)

	var got timelineItem
	if err := graphqljson.UnmarshalData(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify the results manually since cmp.Diff can't handle anonymous structs with embedded fields
	if got.Typename != "ReopenedEvent" {
		t.Errorf("Typename = %v, want %v", got.Typename, "ReopenedEvent")
	}
	if got.ClosedEvent != nil {
		t.Errorf("ClosedEvent = %v, want nil", got.ClosedEvent)
	}
	if got.ReopenedEvent == nil {
		t.Fatal("ReopenedEvent is nil")
	}
	if got.ReopenedEvent.Actor.Login != "user-b" {
		t.Errorf("Actor.Login = %v, want %v", got.ReopenedEvent.Actor.Login, "user-b")
	}
	if got.ReopenedEvent.Actor.Name != "User B" {
		t.Errorf("Actor.Name = %v, want %v", got.ReopenedEvent.Actor.Name, "User B")
	}
	if got.ReopenedEvent.Note != "re-opened" {
		t.Errorf("Note = %v, want %v", got.ReopenedEvent.Note, "re-opened")
	}
	if got.ReopenedEvent.Comment != "shared field" {
		t.Errorf("Comment = %v, want %v", got.ReopenedEvent.Comment, "shared field")
	}
}

func TestUnmarshalGraphQL_typenameWithoutUnion(t *testing.T) {
	t.Parallel()

	type userQuerySimple struct {
		Typename string `json:"__typename"`
		Name     string `json:"name"`
		Age      int    `json:"age"`
	}

	type userQueryWithProfile struct {
		Typename string `json:"__typename"`
		Name     string `json:"name"`
		Profile  *struct {
			Bio string `json:"bio"`
		} `json:"profile"`
	}

	type args struct {
		data []byte
		out  any
	}

	type want struct {
		result any
		err    error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "__typenameがあり、anonymous struct pointer fieldsが0個の場合",
			args: args{
				data: []byte(`{
					"__typename": "User",
					"name": "Alice",
					"age": 30
				}`),
				out: &userQuerySimple{},
			},
			want: want{
				result: &userQuerySimple{
					Typename: "User",
					Name:     "Alice",
					Age:      30,
				},
			},
		},
		{
			name: "__typenameがあり、anonymous struct pointerが1個（非null）の場合",
			args: args{
				data: []byte(`{
					"__typename": "User",
					"name": "Bob",
					"profile": {
						"bio": "Software Engineer"
					}
				}`),
				out: &userQueryWithProfile{},
			},
			want: want{
				result: &userQueryWithProfile{
					Typename: "User",
					Name:     "Bob",
					Profile: &struct {
						Bio string `json:"bio"`
					}{
						Bio: "Software Engineer",
					},
				},
			},
		},
		{
			name: "__typenameがあり、anonymous struct pointerが1個（null）の場合",
			args: args{
				data: []byte(`{
					"__typename": "User",
					"name": "Charlie",
					"profile": null
				}`),
				out: &userQueryWithProfile{},
			},
			want: want{
				result: &userQueryWithProfile{
					Typename: "User",
					Name:     "Charlie",
					Profile:  nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := graphqljson.UnmarshalData(tt.args.data, tt.args.out)

			if diff := cmp.Diff(tt.want.err, err); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if diff := cmp.Diff(tt.want.result, tt.args.out); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

type Number int64

const (
	NumberOne Number = 1
	NumberTwo Number = 2
)

func (n *Number) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return errors.New("enums must be strings")
	}

	switch str {
	case "ONE":
		*n = NumberOne
	case "TWO":
		*n = NumberTwo
	default:
		return fmt.Errorf("number not found Type: %d", n)
	}

	return nil
}

func TestUnmarshalGQL(t *testing.T) {
	t.Parallel()

	type query struct {
		Enum Number
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "enum": "ONE"
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{Enum: NumberOne}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGQL_array(t *testing.T) {
	t.Parallel()

	type query struct {
		Enums []Number
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "enums": ["ONE", "TWO"]
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := query{Enums: []Number{NumberOne, NumberTwo}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGQL_pointer(t *testing.T) {
	t.Parallel()

	type query struct {
		Enum *Number
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "enum": "ONE"
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	v := NumberOne

	want := query{Enum: &v}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGQL_pointerArray(t *testing.T) {
	t.Parallel()

	type query struct {
		Enums []*Number
	}

	var got query

	err := graphqljson.UnmarshalData([]byte(`{
        "enums": ["ONE", "TWO"]
    }`), &got)
	if err != nil {
		t.Fatal(err)
	}

	one := NumberOne
	two := NumberTwo

	want := query{Enums: []*Number{&one, &two}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGQL_pointerArrayReset(t *testing.T) {
	t.Parallel()

	got := []*Number{new(Number)}

	err := graphqljson.UnmarshalData([]byte(`["TWO"]`), &got)
	if err != nil {
		t.Fatal(err)
	}

	want := []*Number{new(Number)}
	*want[0] = NumberTwo

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestIsUnion(t *testing.T) {
	t.Parallel()

	type fragment1 struct {
		Name string
	}
	type fragment2 struct {
		Age int
	}

	tests := []struct {
		name string
		typ  any
		want bool
	}{
		{
			name: "union type with 2 anonymous struct pointers",
			typ: struct {
				Typename string `json:"__typename"`
				A        *struct{ Name string }
				B        *struct{ Value int }
			}{},
			want: true,
		},
		{
			name: "union type with 3 anonymous struct pointers",
			typ: struct {
				Typename string `json:"__typename"`
				A        *struct{ Name string }
				B        *struct{ Value int }
				C        *struct{ Data bool }
			}{},
			want: true,
		},
		{
			name: "not a union - only 1 anonymous struct pointer",
			typ: struct {
				Typename string `json:"__typename"`
				A        *struct{ Name string }
				B        string
			}{},
			want: false,
		},
		{
			name: "not a union - named struct pointers",
			typ: struct {
				Typename string `json:"__typename"`
				A        *Number
				B        *Number
			}{},
			want: false,
		},
		{
			name: "not a union - anonymous embedded fields",
			typ: struct {
				fragment1
				fragment2
			}{},
			want: false,
		},
		{
			name: "not a union - regular struct",
			typ: struct {
				Name string
				Age  int
			}{},
			want: false,
		},
		{
			name: "not a union - anonymous struct pointers with json tags",
			typ: struct {
				QueryType        struct{ Name *string } `json:"queryType"`
				MutationType     *struct{ Name *string } `json:"mutationType"`
				SubscriptionType *struct{ Name *string } `json:"subscriptionType"`
			}{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			typ := reflect.TypeOf(tt.typ)
			got := graphqljson.IsUnion(typ)
			if got != tt.want {
				t.Errorf("IsUnion() = %v, want %v", got, tt.want)
			}
		})
	}
}
