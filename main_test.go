package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	type args struct {
		override string
	}

	type want struct {
		version string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			// -ldflags "-X main.version=..." で埋め込んだ値は、ビルド情報より優先する
			name: "ビルド時に指定したバージョンがあればそれを返す",
			args: args{
				override: "v3.1.0",
			},
			want: want{
				version: "v3.1.0",
			},
		},
		{
			// テストのバイナリにはモジュールのバージョンが記録されないため (devel) になる。
			// ソースに書いた古い定数を返さず、空文字も返さないことを確認する
			name: "指定が無くビルド情報にバージョンが無ければdevelを返す",
			args: args{
				override: "",
			},
			want: want{
				version: "(devel)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveVersion(tt.args.override)

			if diff := cmp.Diff(tt.want.version, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}
