// Package typebind は autobind と Go 型解決のための軽量パッケージローダ。
// gqlgen の binder は NeedSyntax | NeedTypesInfo を含むモードでロードするため
// 対象パッケージのソース構文解析と型検査が走るが、gqlgenc が必要とするのは
// 「import パス + 型名 -> types.Type」の解決だけなので、NeedTypes (export data)
// のみでロードする。
package typebind

import (
	"fmt"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/vektah/gqlparser/v2/ast"
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
			return gqlgenconfig.MapType, nil
		case "any", "interface{}":
			return gqlgenconfig.InterfaceType, nil
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
		return nil, fmt.Errorf("%w: %s.%s", gqlgenconfig.ErrTypeNotFound, pkgPath, typeName)
	}

	t := types.Unalias(obj.Type())
	if sig, ok := t.(*types.Signature); ok {
		return types.Unalias(sig.Params().At(0).Type()), nil
	}

	return t, nil
}

// Autobind は gqlgen の Config.autobind 相当。schema の各型を autobind パッケージ
// から同名 (または Go 命名へ正規化した名前) で探し、見つかれば models に束縛する。
// 加えて "パッケージ名.型名" 短縮形の束縛を import パスへ解決する。
func (b *Binder) Autobind(schema *ast.Schema, models gqlgenconfig.TypeMap, autobindPkgs []string) error {
	if len(autobindPkgs) == 0 {
		return nil
	}
	if err := b.Load(autobindPkgs...); err != nil {
		return err
	}

	for _, t := range schema.Types {
		if models.UserDefined(t.Name) || models[t.Name].ForceGenerate {
			continue
		}
		for _, path := range autobindPkgs {
			pkg, err := b.pkg(path)
			if err != nil {
				return fmt.Errorf("unable to load %s - make sure you're using an import path to a package that exists: %w", path, err)
			}
			obj := lookupAutobindType(pkg, t)
			if obj == nil {
				continue
			}
			models.Add(t.Name, obj.Pkg().Path()+"."+obj.Name())

			break
		}
	}

	// "パッケージ名.型名" 短縮形 (import パスを含まない束縛) を autobind パッケージから解決する
	for name, entry := range models {
		if entry.ForceGenerate {
			continue
		}
		for i, model := range entry.Model {
			pkgName, typeName := pkgAndType(model)
			if pkgName == "" || strings.Contains(pkgName, "/") {
				continue
			}
			for _, path := range autobindPkgs {
				pkg, err := b.pkg(path)
				if err != nil || pkg.Name != pkgName {
					continue
				}
				if obj := pkg.Types.Scope().Lookup(typeName); obj != nil {
					models[name].Model[i] = obj.Pkg().Path() + "." + obj.Name()

					break
				}
			}
		}
	}

	return nil
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
		return nil, fmt.Errorf("package could not be loaded: %s", importPath)
	}

	return pkg, nil
}

// lookupAutobindType はスキーマ型名そのもの、または Go 命名へ正規化した名前で
// パッケージスコープから型を探す。
func lookupAutobindType(pkg *packages.Package, schemaType *ast.Definition) types.Object {
	for _, lookupName := range []string{schemaType.Name, templates.ToGo(schemaType.Name)} {
		if obj := pkg.Types.Scope().Lookup(lookupName); obj != nil {
			return obj
		}
	}

	return nil
}

// pkgAndType は "import/path.TypeName" を import パスと型名に分割する。
func pkgAndType(name string) (string, string) {
	idx := strings.LastIndex(name, ".")
	if idx == -1 {
		return "", name
	}

	return name[:idx], name[idx+1:]
}
