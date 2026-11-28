package config

import (
	"errors"
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
)

// PackageConfig は、生成する Go のファイル 1 つの出力先を表す。
type PackageConfig struct {
	// Filename は、出力するファイルの絶対パス (区切りは /)。空なら、この出力は生成しない。
	Filename string
	// Package は、出力するファイルのパッケージ名。出力先のディレクトリにある Go のファイルの
	// パッケージ名か、ファイルが無ければディレクトリ名。
	Package string
	// ImportPath は、出力先のディレクトリの import パス。go.mod も GOPATH も見つからなければ空。
	ImportPath string
}

// newPackageConfig は、設定に書いたファイル名 filename の出力先を作る。filename が空なら、
// 生成しない出力 (ゼロ値) を返す。
func newPackageConfig(filename string) (PackageConfig, error) {
	if filename == "" {
		return PackageConfig{}, nil
	}
	if !strings.HasSuffix(filename, ".go") {
		return PackageConfig{}, errors.New("filename should be path to a go source file")
	}

	absFilename, err := filepath.Abs(filename)
	if err != nil {
		return PackageConfig{}, fmt.Errorf("resolve %s: %w", filename, err)
	}
	dir := filepath.Dir(absFilename)

	return PackageConfig{
		Filename:   filepath.ToSlash(absFilename),
		Package:    packageNameForDir(dir),
		ImportPath: importPathForDir(dir),
	}, nil
}

// IsDefined は、この出力を生成するかを返す。
func (c PackageConfig) IsDefined() bool {
	return c.Filename != ""
}

// Dir は、出力先のディレクトリを返す。生成しない出力では空文字を返す。
func (c PackageConfig) Dir() string {
	if !c.IsDefined() {
		return ""
	}

	return filepath.Dir(c.Filename)
}

// Pkg は、出力先のパッケージを表す types.Package を返す。生成しない出力では nil を返す。
func (c PackageConfig) Pkg() *types.Package {
	if !c.IsDefined() {
		return nil
	}

	return types.NewPackage(c.ImportPath, c.Package)
}

// invalidPackageNameChar は、パッケージ名に使えない文字に一致する。
var invalidPackageNameChar = regexp.MustCompile(`\W`)

// packageNameForDir は、ディレクトリ dir のパッケージ名を返す。dir にある Go のファイルのうち、名前の順で
// 最初に package 句を読めたファイルのパッケージ名を使う。読めるファイルが無ければ、ディレクトリ名の
// 使えない文字を _ に置き換えた名前を返す。
//
// model は gqlgen の modelgen が生成するため、決め方は gqlgen (internal/code.NameForDir) に合わせる。
func packageNameForDir(dir string) string {
	if entries, err := os.ReadDir(dir); err == nil {
		fset := token.NewFileSet()
		for _, entry := range entries {
			if !strings.HasSuffix(strings.ToLower(entry.Name()), ".go") {
				continue
			}
			if file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.PackageClauseOnly); err == nil {
				return file.Name.Name
			}
		}
	}

	return invalidPackageNameChar.ReplaceAllLiteralString(filepath.Base(dir), "_")
}

// importPathForDir は、絶対パスのディレクトリ dir の import パスを返す。dir から親へたどって最初に
// 見つけた go.mod の module パスに、go.mod のディレクトリからの相対パスを足す。go.mod が無ければ、
// GOPATH の src からの相対パスを返す。どちらでもなければ空文字を返す。
//
// model は gqlgen の modelgen が生成するため、決め方は gqlgen (internal/code.ImportPathForDir) に合わせる。
func importPathForDir(dir string) string {
	for modDir := dir; ; modDir = filepath.Dir(modDir) {
		if goMod, err := os.ReadFile(filepath.Join(modDir, "go.mod")); err == nil {
			modulePath := modfile.ModulePath(goMod)
			if modulePath == "" {
				break
			}

			rel, err := filepath.Rel(modDir, dir)
			if err != nil || rel == "." {
				return modulePath
			}

			return modulePath + "/" + filepath.ToSlash(rel)
		}

		// ルートまでたどった
		if filepath.Dir(modDir) == modDir {
			break
		}
	}

	slashDir := filepath.ToSlash(dir)
	for _, gopath := range filepath.SplitList(build.Default.GOPATH) {
		src := filepath.ToSlash(filepath.Join(gopath, "src"))
		if len(src) < len(slashDir) && strings.EqualFold(src, slashDir[:len(src)]) {
			return slashDir[len(src)+1:]
		}
	}

	return ""
}
