package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"slices"
	"strings"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"

	"github.com/Yamashou/gqlgenc/v3/config"
)

// maxPackageErrorsInMessage は、生成したファイルが読めないときのエラーに載せる、gqlgen がロードした
// パッケージのエラーの数の上限。import 先を読めないパッケージは import の数だけエラーを返すため、
// 先頭だけを載せる。
const maxPackageErrorsInMessage = 3

// validateOutputs は、設定された出力ファイル (model・query・client) が Go のソースとして
// 構文解析できることを確かめる。
//
// gqlgen の templates.Render は、出力の整形に失敗しても「gofmt failed on ...」を標準エラーへ書く
// だけで、整形前の内容をそのまま出力してエラーを返さない。束縛した型を解決できないと
// `*invalid type` のような読めないコードが出るため、ここで失敗にして、呼び出し側が前回の出力を
// 戻せるようにする。原因の手がかりとして、パッケージの読み込みエラーがあればエラーに添える。
//
// 事前条件: 作業ディレクトリが、出力ファイルの相対パスの基準になっている。
func validateOutputs(cfg *config.Config) error {
	outputs := []*gqlgenconfig.PackageConfig{
		&cfg.GQLGenConfig.Model,
		&cfg.GQLGencConfig.QueryGen,
		&cfg.GQLGencConfig.ClientGen,
	}

	for _, output := range outputs {
		if !output.IsDefined() {
			continue
		}

		if err := validateGoFile(output.Filename); err != nil {
			return fmt.Errorf("%w%s", err, packageLoadErrorsNote(cfg))
		}
	}

	return nil
}

// validateGoFile は、filename を Go のソースとして構文解析できなければエラーを返す。
// ファイルが無いときは nil を返す (使用フィルタがすべての型を除いた model ファイルは、
// 生成の後に消してある)。
func validateGoFile(filename string) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("read generated file: %w", err)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.SkipObjectResolution); err != nil {
		return fmt.Errorf("generated file is not valid Go: %w%s", err, firstErrorLineNote(src, err))
	}

	return nil
}

// firstErrorLineNote は、構文エラーの最初の位置にある行を、エラーメッセージの末尾に添える注記に
// して返す。呼び出し側は生成したファイルを消して前回の出力へ戻すため、利用者は位置だけを見ても、
// そこに何が書かれていたかを確かめられない。行が空のときと、位置が分からないときは空文字列を返す。
func firstErrorLineNote(src []byte, parseErr error) string {
	errs, ok := errors.AsType[scanner.ErrorList](parseErr)
	if !ok || len(errs) == 0 {
		return ""
	}

	lineNumber := 0
	for line := range bytes.Lines(src) {
		lineNumber++
		if lineNumber != errs[0].Pos.Line {
			continue
		}

		if text := bytes.TrimSpace(line); len(text) > 0 {
			return fmt.Sprintf("; line %d is: %s", lineNumber, text)
		}

		return ""
	}

	return ""
}

// packageLoadErrorsNote は、束縛に使うパッケージの読み込みエラーを、エラーメッセージの末尾に
// 添える注記にして返す。読み込みエラーが無ければ空文字列を返す。
//
// 読めないコードが生成されるのは、たいてい束縛した型を解決できなかったときである (例: Go の
// コンパイラが書いた export data を golang.org/x/tools が読めず、import 先の型が invalid type に
// なる)。自前の binder (internal/typebind) がロードしたパッケージと、gqlgen がロードした
// パッケージの両方を見る。
func packageLoadErrorsNote(cfg *config.Config) string {
	var msgs []string

	if err := cfg.GQLGencConfig.TypeBinder.LoadErrors(); err != nil {
		msgs = append(msgs, err.Error())
	}

	if cfg.GQLGenConfig.Packages != nil {
		msgs = append(msgs, packageErrorMessages(cfg.GQLGenConfig.Packages.Errors())...)
	}

	if len(msgs) == 0 {
		return ""
	}

	return "; packages were loaded with errors: " + strings.Join(msgs, "; ")
}

// packageErrorMessages は、pkgErrs のメッセージを、重複を除いた昇順で maxPackageErrorsInMessage 件
// まで返す。残りがあれば、その件数を示す 1 件を末尾に足す。
//
// gqlgen は、ロードしたパッケージのエラーを map の反復順で返す。並べ替えて、同じ状態からは
// 同じメッセージになるようにする。
func packageErrorMessages(pkgErrs []error) []string {
	msgs := make([]string, 0, len(pkgErrs))
	for _, pkgErr := range pkgErrs {
		msgs = append(msgs, pkgErr.Error())
	}

	slices.Sort(msgs)
	msgs = slices.Compact(msgs)

	if rest := len(msgs) - maxPackageErrorsInMessage; rest > 0 {
		msgs = append(msgs[:maxPackageErrorsInMessage], fmt.Sprintf("and %d more", rest))
	}

	return msgs
}
