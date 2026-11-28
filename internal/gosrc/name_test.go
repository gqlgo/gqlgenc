package gosrc

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/99designs/gqlgen/codegen/templates"
)

func TestToGo(t *testing.T) {
	t.Parallel()

	type args struct {
		name string
	}

	type want struct {
		goName string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		// 語の分け方
		{name: "大文字のスネークケースは語の頭だけ大文字にする", args: args{name: "TO_CAMEL"}, want: want{goName: "ToCamel"}},
		{name: "小文字のスネークケースは語の頭を大文字にする", args: args{name: "to_camel"}, want: want{goName: "ToCamel"}},
		{name: "キャメルケースは先頭を大文字にする", args: args{name: "toCamel"}, want: want{goName: "ToCamel"}},
		{name: "パスカルケースはそのまま", args: args{name: "ToCamel"}, want: want{goName: "ToCamel"}},
		{name: "ハイフンは区切りとして扱う", args: args{name: "to-camel"}, want: want{goName: "ToCamel"}},
		{name: "先頭のハイフンは捨てる", args: args{name: "-to-camel"}, want: want{goName: "ToCamel"}},
		{name: "先頭のアンダースコアは捨てる", args: args{name: "_to-camel"}, want: want{goName: "ToCamel"}},
		{name: "空白は区切りとして扱う", args: args{name: "to camel"}, want: want{goName: "ToCamel"}},
		{name: "区切りが続いても1つの区切りとして扱う", args: args{name: "to__camel"}, want: want{goName: "ToCamel"}},
		{name: "1文字の名前は大文字にする", args: args{name: "a"}, want: want{goName: "A"}},
		{name: "大文字1文字の名前はそのまま", args: args{name: "A"}, want: want{goName: "A"}},
		{name: "大文字から始まる語の途中では分けない", args: args{name: "ITicket"}, want: want{goName: "ITicket"}},
		{name: "小文字から大文字への変わり目で分ける", args: args{name: "fooTicket"}, want: want{goName: "FooTicket"}},
		{name: "複数の語のスネークケース", args: args{name: "USER_IDENTITY"}, want: want{goName: "UserIdentity"}},
		{name: "複数の語のキャメルケース", args: args{name: "userIdentity"}, want: want{goName: "UserIdentity"}},

		// 頭字語
		{name: "小文字の頭字語は大文字にする", args: args{name: "id"}, want: want{goName: "ID"}},
		{name: "大文字の頭字語はそのまま", args: args{name: "ID"}, want: want{goName: "ID"}},
		{name: "語の途中の頭字語を大文字にする", args: args{name: "user_id"}, want: want{goName: "UserID"}},
		{name: "末尾が頭字語の名前はそのまま", args: args{name: "FooID"}, want: want{goName: "FooID"}},
		{name: "先頭が頭字語の名前は頭字語の後ろで分ける", args: args{name: "IDFoo"}, want: want{goName: "IDFoo"}},
		{name: "末尾が長い頭字語の名前はそのまま", args: args{name: "FooASCII"}, want: want{goName: "FooASCII"}},
		{name: "先頭が長い頭字語の名前はそのまま", args: args{name: "ASCIIFoo"}, want: want{goName: "ASCIIFoo"}},
		{name: "数字を含む頭字語が末尾の名前はそのまま", args: args{name: "FooUTF8"}, want: want{goName: "FooUTF8"}},
		{name: "数字を含む頭字語が先頭の名前はそのまま", args: args{name: "UTF8Foo"}, want: want{goName: "UTF8Foo"}},
		{name: "頭字語の後ろに語が続く名前はそのまま", args: args{name: "JSONEncoding"}, want: want{goName: "JSONEncoding"}},
		{name: "頭字語の複数形は1語として扱う", args: args{name: "RelatedURLs"}, want: want{goName: "RelatedURLs"}},
		{name: "IDの複数形は1語として扱う", args: args{name: "ImageIDs"}, want: want{goName: "ImageIDs"}},
		{name: "頭字語と同じつづりでも小文字が混ざれば頭字語にしない", args: args{name: "RelatedUrls"}, want: want{goName: "RelatedUrls"}},
		{name: "Idで終わるキャメルケースは頭字語にする", args: args{name: "GetUserId"}, want: want{goName: "GetUserID"}},

		// ID・IP から始まる語は、頭字語として分けない
		{name: "IDから始まる大文字の語は分けない", args: args{name: "IDLE"}, want: want{goName: "Idle"}},
		{name: "Idから始まる語は分けない", args: args{name: "Idle"}, want: want{goName: "Idle"}},
		{name: "idから始まる小文字の語は分けない", args: args{name: "idle"}, want: want{goName: "Idle"}},
		{name: "IDから始まる長い大文字の語は分けない", args: args{name: "IDENTITIES"}, want: want{goName: "Identities"}},
		{name: "IPから始まる大文字の語は分けない", args: args{name: "IPHONE"}, want: want{goName: "Iphone"}},
		{name: "小文字1文字の後ろに大文字が続く語", args: args{name: "iPHONE"}, want: want{goName: "IPhone"}},

		// 境界値
		{name: "アンダースコアだけの名前はそのまま返す", args: args{name: "_"}, want: want{goName: "_"}},
		{name: "空文字は空文字を返す", args: args{name: ""}, want: want{goName: ""}},
		{name: "区切りだけの名前は空文字を返す", args: args{name: "__"}, want: want{goName: ""}},
		{name: "先頭のアンダースコア2つは捨てる", args: args{name: "__typename"}, want: want{goName: "Typename"}},
		{name: "先頭のアンダースコアを捨てて頭字語にする", args: args{name: "_id"}, want: want{goName: "ID"}},
		{name: "数字と数字の間の区切りは残す", args: args{name: "v1_2"}, want: want{goName: "V1_2"}},
		{name: "数字と文字の間の区切りは捨てる", args: args{name: "v1_beta"}, want: want{goName: "V1Beta"}},
		{name: "ASCII でない文字を含む名前", args: args{name: "ユーザー_id"}, want: want{goName: "ユーザーID"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ToGo(tt.args.name)

			if diff := cmp.Diff(tt.want.goName, got); diff != "" {
				t.Errorf("goName diff(-want +got): %s", diff)
			}

			// model は gqlgen の modelgen が生成するため、gqlgen と同じ名前にならなければならない
			if diff := cmp.Diff(templates.ToGo(tt.args.name), got); diff != "" {
				t.Errorf("gqlgen の templates.ToGo との diff(-want +got): %s", diff)
			}
		})
	}
}
