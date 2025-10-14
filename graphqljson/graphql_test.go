package graphqljson_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"encoding/json/jsontext"
	"github.com/google/go-cmp/cmp"

	"github.com/Yamashou/gqlgenc/v3/graphqljson"
)

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

func (i *issueTimelineItem) UnmarshalJSON(data []byte) error {
	type meta struct {
		Typename string `json:"__typename"`
	}

	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("decode union metadata: %w", err)
	}
	if m.Typename == "" {
		return fmt.Errorf("__typename is required for issueTimelineItem union")
	}

	i.Typename = m.Typename
	i.ClosedEvent = nil
	i.ReopenedEvent = nil

	switch m.Typename {
	case "ClosedEvent":
		var payload struct {
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("decode ClosedEvent: %w", err)
		}
		i.ClosedEvent = &payload
	case "ReopenedEvent":
		var payload struct {
			Actor struct {
				Login string `json:"login"`
			} `json:"actor"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("decode ReopenedEvent: %w", err)
		}
		i.ReopenedEvent = &payload
	}

	return nil
}

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

type fragmentUser struct {
	Name string `json:"name"`
}

type fragment1 struct {
	Name string `json:"name"`
}

type fragment2 struct {
	Name string       `json:"name"`
	User fragmentUser `json:"user"`
}

type fragmentQuery struct {
	Typename      string `json:"__typename"`
	Name          string `json:"name"`
	UserFragment1 fragment1
	UserFragment2 fragment2
}

func (q *fragmentQuery) UnmarshalJSON(data []byte) error {
	var aux struct {
		Typename string       `json:"__typename"`
		Name     string       `json:"name"`
		User     fragmentUser `json:"user"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	q.Typename = aux.Typename
	q.Name = aux.Name
	q.UserFragment1 = fragment1{Name: aux.Name}
	q.UserFragment2 = fragment2{Name: aux.Name, User: aux.User}

	return nil
}

func (t *timelineItem) UnmarshalJSON(data []byte) error {
	type meta struct {
		Typename string `json:"__typename"`
	}

	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("decode union metadata: %w", err)
	}
	if m.Typename == "" {
		return fmt.Errorf("__typename is required for timelineItem union")
	}

	t.Typename = m.Typename
	t.ClosedEvent = nil
	t.ReopenedEvent = nil

	switch m.Typename {
	case "ClosedEvent":
		var payload struct {
			Actor actor `json:"actor"`
			Extra string
			sharedFragment
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("decode ClosedEvent: %w", err)
		}
		t.ClosedEvent = &payload
	case "ReopenedEvent":
		var payload struct {
			Actor actor `json:"actor"`
			Note  string
			sharedFragment
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("decode ReopenedEvent: %w", err)
		}
		t.ReopenedEvent = &payload
	}

	return nil
}

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
		Foo []string `json:"foo"`
		Bar []string `json:"bar"`
		Baz []string `json:"baz"`
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
			Name string `json:"name"`
		} `json:"foo"`
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
		Foo: []struct {
			Name string `json:"name"`
		}{
			{Name: "bar"},
			{Name: "baz"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_pointer(t *testing.T) {
	t.Parallel()

	type query struct {
		Foo *string `json:"foo"`
		Bar *string `json:"bar"`
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
			Name string `json:"name"`
		} `json:"foo"`
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
		Foo: []*struct {
			Name string `json:"name"`
		}{
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
	want := "decode graphql data: decode json: json: cannot unmarshal JSON object into Go graphqljson_test.query: Go struct has no exported fields"
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

	if got, want := err.Error(), "decode graphql data: decode json: jsontext: invalid character '{' after top-level value after offset 14"; got != want {
		t.Errorf("got error: %v, want: %v", got, want)
	}
}

func TestUnmarshalGraphQL_unknownJSONValue(t *testing.T) {
	t.Parallel()

	type query struct {
		Unknown jsontext.Value `json:"extra"`
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
		Outputs jsontext.Value `json:"outputs"`
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

	var got fragmentQuery

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

	want := fragmentQuery{
		Typename:      "User",
		Name:          "John Doe",
		UserFragment1: fragment1{Name: "John Doe"},
		UserFragment2: fragment2{
			Name: "John Doe",
			User: fragmentUser{Name: "Nested John"},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff(-want +got): %s", diff)
	}
}

func TestUnmarshalGraphQL_unionStruct(t *testing.T) {
	t.Parallel()

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

func (n *Number) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		switch str {
		case "ONE":
			*n = NumberOne
		case "TWO":
			*n = NumberTwo
		default:
			return fmt.Errorf("number not found Type: %s", str)
		}
		return nil
	}

	var num int64
	if err := json.Unmarshal(data, &num); err == nil {
		*n = Number(num)
		return nil
	}

	return fmt.Errorf("unsupported enum representation: %s", string(data))
}

func TestUnmarshalGQL(t *testing.T) {
	t.Parallel()

	type query struct {
		Enum Number `json:"enum"`
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
		Enums []Number `json:"enums"`
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
		Enum *Number `json:"enum"`
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
		Enums []*Number `json:"enums"`
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
