package client

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestMergeJSONObject(t *testing.T) {
	t.Parallel()

	type fields struct {
		dstJSON string
	}

	type args struct {
		v any
	}

	type want struct {
		dst map[string]string
		err error
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "新しいキーがそのまま追加される",
			fields: fields{
				dstJSON: `{}`,
			},
			args: args{
				v: struct {
					Name string `json:"name"`
				}{Name: "Alice"},
			},
			want: want{
				dst: map[string]string{
					"name": `"Alice"`,
				},
			},
		},
		{
			name: "重複するスカラーキーは既存値が維持される（先勝ち）",
			fields: fields{
				dstJSON: `{"name":"existing"}`,
			},
			args: args{
				v: struct {
					Name string `json:"name"`
				}{Name: "new"},
			},
			want: want{
				dst: map[string]string{
					"name": `"existing"`,
				},
			},
		},
		{
			name: "重複するオブジェクトキーは再帰的にマージされる",
			fields: fields{
				dstJSON: `{"profile":{"id":"1"}}`,
			},
			args: args{
				v: struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				}{
					Profile: struct {
						Name string `json:"name"`
					}{Name: "Alice"},
				},
			},
			want: want{
				dst: map[string]string{
					"profile": `{"id":"1","name":"Alice"}`,
				},
			},
		},
		{
			name: "重複する同じ長さの配列は要素ごとにマージされる",
			fields: fields{
				dstJSON: `{"items":[{"id":"1"}]}`,
			},
			args: args{
				v: struct {
					Items []struct {
						Name string `json:"name"`
					} `json:"items"`
				}{
					Items: []struct {
						Name string `json:"name"`
					}{{Name: "A"}},
				},
			},
			want: want{
				dst: map[string]string{
					"items": `[{"id":"1","name":"A"}]`,
				},
			},
		},
		{
			name: "長さの異なる配列は既存値が維持される",
			fields: fields{
				dstJSON: `{"items":[{"id":"1"},{"id":"2"}]}`,
			},
			args: args{
				v: struct {
					Items []struct {
						Name string `json:"name"`
					} `json:"items"`
				}{
					Items: []struct {
						Name string `json:"name"`
					}{{Name: "A"}},
				},
			},
			want: want{
				dst: map[string]string{
					"items": `[{"id":"1"},{"id":"2"}]`,
				},
			},
		},
		{
			name: "オブジェクトにエンコードされない値はエラー",
			fields: fields{
				dstJSON: `{}`,
			},
			args: args{
				v: "not an object",
			},
			want: want{
				err: cmpopts.AnyError,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dst := mustUnmarshalJSONObject(t, tt.fields.dstJSON)
			err := MergeJSONObject(dst, tt.args.v)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}

			if tt.want.err != nil {
				return
			}

			if diff := cmp.Diff(tt.want.dst, jsonValuesToStrings(dst)); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func mustUnmarshalJSONObject(t *testing.T, jsonStr string) map[string]jsontext.Value {
	t.Helper()

	var dst map[string]jsontext.Value
	if err := json.Unmarshal([]byte(jsonStr), &dst); err != nil {
		t.Fatalf("setup dst: %v", err)
	}
	return dst
}

func jsonValuesToStrings(m map[string]jsontext.Value) map[string]string {
	out := make(map[string]string, len(m))
	for key, value := range m {
		out[key] = string(value)
	}
	return out
}
