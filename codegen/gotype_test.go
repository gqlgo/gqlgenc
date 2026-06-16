package codegen

import (
	gotypes "go/types"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
)

func TestFieldTypeName(t *testing.T) {
	t.Parallel()

	type args struct {
		parentTypeName string
		fieldName      string
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
			// 親型名の先頭を大文字にし、フィールド名を Go の公開名に変換して結合する
			name: "親型名とフィールド名から公開型名を生成する",
			args: args{
				parentTypeName: "userOperation",
				fieldName:      "article",
			},
			want: want{
				typeName: "UserOperation_Article",
			},
		},
		{
			// 親型名が空 (インラインフラグメント) でも firstUpper が panic せず "_<Field>" になる
			name: "親型名が空でもfirstUpperが安全に動く",
			args: args{
				parentTypeName: "",
				fieldName:      "article",
			},
			want: want{
				typeName: "_Article",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fieldTypeName(tt.args.parentTypeName, tt.args.fieldName)

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

func TestResolveModelType(t *testing.T) {
	t.Parallel()

	type args struct {
		binder   *gqlgenconfig.Binder
		models   gqlgenconfig.TypeMap
		typeName string
		nonNull  bool
	}

	type want struct {
		goType string
		err    error
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// UC1 (gqlgen.model 未指定) で enum/input が未束縛のとき、panic ではなくエラーを返す
			name: "束縛が無いGraphQL型はpanicせずエラーを返す",
			args: args{
				models:   gqlgenconfig.TypeMap{},
				typeName: "Status",
				nonNull:  true,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// キーは在るが Model が空のときも未束縛として同じくエラーを返す
			name: "Modelが空のGraphQL型はエラーを返す",
			args: args{
				models:   gqlgenconfig.TypeMap{"Status": gqlgenconfig.TypeMapEntry{}},
				typeName: "Status",
				nonNull:  false,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// 束縛は在るが指す Go 型を binder が解決できないときもエラーを返す
			// (@goModel(model: "誤り") 相当)。パッケージ区切りの無い束縛名なら FindObject が
			// pkgs を参照せず失敗するため、最小構成の binder (nil pkgs) でも安全に到達する。
			name: "束縛先のGo型を解決できないときはエラーを返す",
			args: args{
				binder:   (&gqlgenconfig.Config{}).NewBinder(),
				models:   gqlgenconfig.TypeMap{"Status": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"NoPackageSeparator"}}},
				typeName: "Status",
				nonNull:  true,
			},
			want: want{
				goType: "invalid type",
				err:    cmpopts.AnyError,
			},
		},
		{
			// 束縛が解決できるときはその Go 型を返す。nonNull なのでポインタで包まない。
			// map[string]any は FindType の特殊扱いで pkgs を参照せず解決できる。
			name: "束縛が解決できるときはその型を返す",
			args: args{
				binder:   (&gqlgenconfig.Config{}).NewBinder(),
				models:   gqlgenconfig.TypeMap{"Meta": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"map[string]any"}}},
				typeName: "Meta",
				nonNull:  true,
			},
			want: want{
				goType: "map[string]any",
			},
		},
		{
			// nullable のときは解決した型をポインタで包む。
			name: "nullableのときは解決した型をポインタで包む",
			args: args{
				binder:   (&gqlgenconfig.Config{}).NewBinder(),
				models:   gqlgenconfig.TypeMap{"Meta": gqlgenconfig.TypeMapEntry{Model: gqlgenconfig.StringList{"map[string]any"}}},
				typeName: "Meta",
				nonNull:  false,
			},
			want: want{
				goType: "*map[string]any",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveModelType(tt.args.binder, tt.args.models, tt.args.typeName, tt.args.nonNull)

			if diff := cmp.Diff(tt.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("error diff(-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.want.goType, got.String()); diff != "" {
				t.Errorf("goType diff(-want +got): %s", diff)
			}
		})
	}
}
