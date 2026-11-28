// Package typebind は autobind と Go 型解決のための軽量パッケージローダ。
// gqlgen の binder は NeedSyntax | NeedTypesInfo を含むモードでロードするため
// 対象パッケージのソース構文解析と型検査が走るが、gqlgenc が必要とするのは
// 「import パス + 型名 -> types.Type」の解決だけなので、NeedTypes (export data)
// のみでロードする。
package typebind

import (
	"errors"
	"fmt"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/Yamashou/gqlgenc/v3/internal/gosrc"
)

// ErrTypeNotFound は、束縛先のパッケージに、指定した名前の型が無いことを表す。
var ErrTypeNotFound = errors.New("unable to find type")

// パッケージを持たない束縛 ("map[string]any"・"any") の Go 型
var (
	mapType = types.NewMap(types.Typ[types.String], types.Universe.Lookup("any").Type())
	anyType = types.Universe.Lookup("any").Type()
)

// loadMode は export data から型情報だけを読むモード。NeedSyntax を含めないことで
// x/tools はソースを型検査せず、go list -export が生成した export data を使う。
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedTypes

// Binder は import パスごとのロード済みパッケージを保持する。スレッドセーフではない。
type Binder struct {
	pkgs      map[string]*packages.Package
	dirToPath map[string]string
	fallback  func(importPath string) *packages.Package
}

func New() *Binder {
	return &Binder{pkgs: map[string]*packages.Package{}, dirToPath: map[string]string{}}
}

// SetFallback は自前キャッシュに無いパッケージの解決先を設定する。gqlgen の
// Packages.LoadWithTypes を渡すことで、gqlgen が Init でロード済みのパッケージ
// (graphql / introspection / named 束縛先) を二重ロードせずに再利用する。
func (b *Binder) SetFallback(fallback func(importPath string) *packages.Package) {
	b.fallback = fallback
}

// Evict は importPath のパッケージをキャッシュから外す。EvictDir と違い、
// ロード失敗の負キャッシュ (nil エントリ) も外せる。生成でファイルが書き換わった
// パッケージを、同一 config 内の後続処理が生成後の状態で再ロードするために使う。
func (b *Binder) Evict(importPath string) {
	pkg, ok := b.pkgs[importPath]
	if !ok {
		return
	}
	delete(b.pkgs, importPath)
	if pkg != nil && pkg.Dir != "" {
		delete(b.dirToPath, pkg.Dir)
	}
}

// EvictDir は dir のパッケージをキャッシュから外す。生成でファイルが書き換わった
// パッケージを、後続の config が束縛するときに生成後の状態で再ロードさせるため。
func (b *Binder) EvictDir(dir string) {
	path, ok := b.dirToPath[dir]
	if !ok {
		return
	}
	delete(b.pkgs, path)
	delete(b.dirToPath, dir)
}

// Load は未ロードの import パスをまとめて1回の go list でロードしてキャッシュする。
func (b *Binder) Load(importPaths ...string) error {
	missing := make([]string, 0, len(importPaths))
	seen := make(map[string]bool, len(importPaths))
	for _, path := range importPaths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		if _, ok := b.pkgs[path]; !ok {
			missing = append(missing, path)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	pkgs, err := packages.Load(&packages.Config{Mode: loadMode}, missing...)
	if err != nil {
		return fmt.Errorf("load packages %v: %w", missing, err)
	}
	for _, pkg := range pkgs {
		b.pkgs[pkg.PkgPath] = pkg
		if pkg.Dir != "" {
			b.dirToPath[pkg.Dir] = pkg.PkgPath
		}
	}
	// 見つからなかったパスにも nil を記録して再ロードを防ぐ
	for _, path := range missing {
		if _, ok := b.pkgs[path]; !ok {
			b.pkgs[path] = nil
		}
	}

	return nil
}

// FindTypeFromName は "import/path.TypeName" 形式の束縛文字列を types.Type に解決する。
func (b *Binder) FindTypeFromName(name string) (types.Type, error) {
	pkgPath, typeName := pkgAndType(name)

	return b.FindType(pkgPath, typeName)
}

// FindType は gqlgen の Binder.FindType と同じ規則で型を解決する。
// Marshal<TypeName> 関数が存在する場合はその第1引数の型を優先する
// (graphql.String などの関数ベース marshaler の束縛)。
func (b *Binder) FindType(pkgPath, typeName string) (types.Type, error) {
	if pkgPath == "" {
		switch typeName {
		case "map[string]any", "map[string]interface{}":
			return mapType, nil
		case "any", "interface{}":
			return anyType, nil
		default:
			return nil, fmt.Errorf("package cannot be empty for type: %s", typeName)
		}
	}

	pkg, err := b.pkg(pkgPath)
	if err != nil {
		return nil, err
	}

	scope := pkg.Types.Scope()
	obj := scope.Lookup("Marshal" + typeName)
	if obj == nil {
		obj = scope.Lookup(typeName)
	}
	if obj == nil {
		// 読み込みエラーのあるパッケージは型を 1 つも持たないため、型が無い原因として添える
		return nil, fmt.Errorf("%w: %s.%s%s", ErrTypeNotFound, pkgPath, typeName, loadErrorsNote(pkg))
	}

	t := types.Unalias(obj.Type())
	if sig, ok := t.(*types.Signature); ok {
		return types.Unalias(sig.Params().At(0).Type()), nil
	}

	return t, nil
}

// AutobindType は、スキーマの型 schemaTypeName と同じ名前 (または Go の名前へ直した名前) の Go 型を
// autobind パッケージから順に探し、最初に見つけた型を "import/path.TypeName" の形で返す。
// どのパッケージにも無ければ空文字を返す。探し方は gqlgen の Config.autobind と同じである。
func (b *Binder) AutobindType(schemaTypeName string, autobindPkgs []string) (string, error) {
	for _, path := range autobindPkgs {
		pkg, err := b.pkg(path)
		if err != nil {
			return "", fmt.Errorf("unable to load %s - make sure you're using an import path to a package that exists: %w", path, err)
		}

		for _, lookupName := range []string{schemaTypeName, gosrc.ToGo(schemaTypeName)} {
			if obj := pkg.Types.Scope().Lookup(lookupName); obj != nil {
				return obj.Pkg().Path() + "." + obj.Name(), nil
			}
		}
	}

	return "", nil
}

// QualifyShortBinding は、"パッケージ名.型名" の短縮形の束縛 binding を autobind パッケージから探し、
// "import/path.TypeName" の形にして返す。binding が短縮形でないとき (import パスを含む・パッケージ名が
// 無い) と、当てはまる型が無いときは、false を返す。
func (b *Binder) QualifyShortBinding(binding string, autobindPkgs []string) (string, bool) {
	pkgName, typeName := pkgAndType(binding)
	if pkgName == "" || strings.Contains(pkgName, "/") {
		return "", false
	}

	for _, path := range autobindPkgs {
		pkg, err := b.pkg(path)
		if err != nil || pkg.Name != pkgName {
			continue
		}
		if obj := pkg.Types.Scope().Lookup(typeName); obj != nil {
			return obj.Pkg().Path() + "." + obj.Name(), true
		}
	}

	return "", false
}

func (b *Binder) pkg(importPath string) (*packages.Package, error) {
	pkg, ok := b.pkgs[importPath]
	if !ok && b.fallback != nil {
		pkg = b.fallback(importPath)
		b.pkgs[importPath] = pkg
		if pkg != nil && pkg.Dir != "" {
			b.dirToPath[pkg.Dir] = pkg.PkgPath
		}
		ok = true
	}
	if !ok {
		if err := b.Load(importPath); err != nil {
			return nil, err
		}
		pkg = b.pkgs[importPath]
	}
	if pkg == nil || pkg.Types == nil {
		return nil, fmt.Errorf("package could not be loaded: %s%s", importPath, loadErrorsNote(pkg))
	}

	return pkg, nil
}

// pkgAndType は "import/path.TypeName" を import パスと型名に分割する。
func pkgAndType(name string) (string, string) {
	pkgPath, typeName, found := strings.CutLast(name, ".")
	if !found {
		return "", name
	}

	return pkgPath, typeName
}
