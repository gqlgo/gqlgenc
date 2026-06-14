package codegen

import (
	gotypes "go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFieldTypeName(t *testing.T) {
	t.Parallel()

	type args struct {
		parentTypeName  string
		fieldName       string
		exportQueryType bool
	}

	type want struct {
		typeName string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// export_query_type が true のときは親型名の先頭を大文字にして公開型名にする
			name: "export_query_typeがtrueのとき先頭大文字の公開型名になる",
			args: args{
				parentTypeName:  "userOperation",
				fieldName:       "article",
				exportQueryType: true,
			},
			want: want{
				typeName: "UserOperation_Article",
			},
		},
		{
			// export_query_type が false (デフォルト) のときは親型名の先頭を小文字にして非公開型名にする
			name: "export_query_typeがfalseのとき先頭小文字の非公開型名になる",
			args: args{
				parentTypeName:  "userOperation",
				fieldName:       "article",
				exportQueryType: false,
			},
			want: want{
				typeName: "userOperation_Article",
			},
		},
		{
			// 親型名が空 (インラインフラグメント) でも firstLower が panic せず "_<Field>" になる
			name: "親型名が空でもfirstLowerが安全に動く",
			args: args{
				parentTypeName:  "",
				fieldName:       "article",
				exportQueryType: false,
			},
			want: want{
				typeName: "_Article",
			},
		},
		{
			// 親型名が空でも firstUpper が panic せず "_<Field>" になる
			name: "親型名が空でもfirstUpperが安全に動く",
			args: args{
				parentTypeName:  "",
				fieldName:       "article",
				exportQueryType: true,
			},
			want: want{
				typeName: "_Article",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fieldTypeName(tt.args.parentTypeName, tt.args.fieldName, tt.args.exportQueryType)

			if diff := cmp.Diff(tt.want.typeName, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestGoTypeName(t *testing.T) {
	t.Parallel()

	named := gotypes.NewNamed(gotypes.NewTypeName(0, nil, "Foo", nil), gotypes.NewStruct(nil, nil), nil)

	type args struct {
		typ gotypes.Type
	}

	type want struct {
		name string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// named 型はその型名を返す
			name: "named型は型名を返す",
			args: args{
				typ: named,
			},
			want: want{
				name: "Foo",
			},
		},
		{
			// ポインタ型は要素を辿って型名を返す
			name: "ポインタ型は要素の型名を返す",
			args: args{
				typ: gotypes.NewPointer(named),
			},
			want: want{
				name: "Foo",
			},
		},
		{
			// named でもポインタでもない型は空文字を返す
			name: "namedでもポインタでもない型は空文字を返す",
			args: args{
				typ: gotypes.Typ[gotypes.String],
			},
			want: want{
				name: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := goTypeName(tt.args.typ)

			if diff := cmp.Diff(tt.want.name, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
