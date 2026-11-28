package typebind

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// maxLoadErrorsPerPackage は、1 つのパッケージについてエラーメッセージに載せる読み込みエラーの数の上限。
// コンパイルできないパッケージは同じ原因のエラーを何件も返すことがあるため、先頭だけを載せる。
const maxLoadErrorsPerPackage = 3

// maxLoadErrorPackages は、エラーメッセージに載せる、読み込みエラーのあるパッケージの数の上限。
// 原因が 1 つでも (例: export data を読めない)、ロードしたパッケージのすべてがエラーになる
// ことがあるため、先頭だけを載せる。
const maxLoadErrorPackages = 3

// LoadErrors は、ロードしたパッケージの読み込みエラーを 1 つのエラーにまとめて返す。
// 読み込みエラーのあるパッケージが無ければ nil を返す。b が nil のときも nil を返す。
//
// loadMode は export data から型を読むため、コンパイルできないパッケージや、ビルド制約で
// 全ファイルが外れるパッケージは、型を 1 つも持たない空のパッケージとしてロードされる。その
// パッケージの型はどれも束縛されず、呼び出し側には「束縛先に型が無い」という設定の不備と
// 同じに見える。型を束縛できなかったときに、原因の候補としてエラーへ添えるために使う。
//
// 事後条件: パッケージは import パスの昇順に並び、maxLoadErrorPackages 個までを載せる
// (同じ状態からは同じメッセージになる)。残りは個数で示す。
func (b *Binder) LoadErrors() error {
	if b == nil {
		return nil
	}

	paths := make([]string, 0, len(b.pkgs))
	for path, pkg := range b.pkgs {
		if pkg != nil && len(pkg.Errors) > 0 {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	slices.Sort(paths)

	shown := paths[:min(len(paths), maxLoadErrorPackages)]
	msgs := make([]string, 0, len(shown)+1)
	for _, path := range shown {
		msgs = append(msgs, packageLoadErrors(b.pkgs[path]))
	}
	if rest := len(paths) - len(shown); rest > 0 {
		msgs = append(msgs, fmt.Sprintf("and %d more package(s)", rest))
	}

	return errors.New(strings.Join(msgs, "; "))
}

// packageLoadErrors は、pkg の読み込みエラーを 1 つの文にまとめる。エラーは
// maxLoadErrorsPerPackage 件までを載せ、残りは件数で示す。
//
// 事前条件: pkg は nil でなく、pkg.Errors は空でない。
func packageLoadErrors(pkg *packages.Package) string {
	shown := pkg.Errors[:min(len(pkg.Errors), maxLoadErrorsPerPackage)]
	msgs := make([]string, 0, len(shown))
	for _, loadErr := range shown {
		msgs = append(msgs, loadErrorMessage(loadErr))
	}

	rest := ""
	if n := len(pkg.Errors) - len(shown); n > 0 {
		rest = fmt.Sprintf(" (and %d more)", n)
	}

	return fmt.Sprintf("package %s was loaded with errors: %s%s", pkg.PkgPath, strings.Join(msgs, ", "), rest)
}

// loadErrorMessage は、位置の分かるエラーには位置を付けたメッセージを返す。packages.Error の
// Error メソッドは位置が無いときに "-" を付けるため、使わない。
func loadErrorMessage(loadErr packages.Error) string {
	if loadErr.Pos == "" {
		return loadErr.Msg
	}

	return loadErr.Pos + ": " + loadErr.Msg
}

// loadErrorsNote は、pkg に読み込みエラーがあれば、エラーメッセージの末尾に添える注記を返す。
// 無ければ空文字列を返す。
func loadErrorsNote(pkg *packages.Package) string {
	if pkg == nil || len(pkg.Errors) == 0 {
		return ""
	}

	return ": " + packageLoadErrors(pkg)
}
