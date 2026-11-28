package gosrc

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
)

// Imports は、生成する Go のファイル 1 つが import するパッケージを集める。
type Imports struct {
	// destPkgPath は、生成するファイルのパッケージの import パス。このパッケージは import しない
	destPkgPath string
	// specs は、加えた順の import
	specs []*importSpec
}

// importSpec は、import 1 つ。
type importSpec struct {
	// path は import パス
	path string
	// name はパッケージ名
	name string
	// alias は、生成するファイルの中でこのパッケージを指す名前。ふつうは name と同じで、
	// 他の import と重なるときは name に番号を付けた名前になる
	alias string
}

// NewImports は、import パス destPkgPath のパッケージに生成するファイルの Imports を作る。
func NewImports(destPkgPath string) *Imports {
	return &Imports{destPkgPath: destPkgPath, specs: nil}
}

// Reserve は、パッケージ path を、パッケージ名 name で参照する import として加える。生成するコードに
// パッケージ名を直接書いて参照するパッケージに使い、Qualifier より先に呼ぶ。
//
// 生成するファイルと同じパッケージは加えない。同じパッケージが別の名前で、または別のパッケージが
// 同じ名前で既にあるときは、エラーを返す。
func (s *Imports) Reserve(path, name string) error {
	if path == s.destPkgPath {
		return nil
	}

	if existing := s.findByPath(path); existing != nil {
		if existing.alias == name {
			return nil
		}

		return fmt.Errorf("import %q is already imported as %s", path, existing.alias)
	}
	if existing := s.findByAlias(name); existing != nil {
		return fmt.Errorf("import name %s of %q is already used by %q", name, path, existing.path)
	}

	s.specs = append(s.specs, &importSpec{path: path, name: name, alias: name})

	return nil
}

// Qualifier は、パッケージ pkg の名前を参照するときに前へ付ける名前を返す。まだ import していなければ
// 加える。パッケージ名が他の import と重なるときは、番号を付けた別名 (domain1 など) にする。
// 生成するファイルと同じパッケージには、空文字を返す。types.TypeString の修飾に使える。
func (s *Imports) Qualifier(pkg *types.Package) string {
	path := trimVendor(pkg.Path())
	if path == "" || path == s.destPkgPath {
		return ""
	}

	if existing := s.findByPath(path); existing != nil {
		return existing.alias
	}

	alias := pkg.Name()
	for i := 1; s.findByAlias(alias) != nil; i++ {
		alias = pkg.Name() + strconv.Itoa(i)
	}
	s.specs = append(s.specs, &importSpec{path: path, name: pkg.Name(), alias: alias})

	return alias
}

// TypeString は、型 t を生成するファイルに書くときの表記を返す。型のパッケージを import に加え、
// その import の名前で修飾する。go/types は空の interface を interface{} と書くため、any に揃える
// (空でない interface は interface{M()} の形になるので、置換の対象にならない)。
func (s *Imports) TypeString(t types.Type) string {
	return strings.ReplaceAll(types.TypeString(t, s.Qualifier), "interface{}", "any")
}

// lines は、import 宣言の中に書く行を返す。名前が used に無い import は書かない。
func (s *Imports) lines(used map[string]bool) string {
	var lines strings.Builder
	for _, spec := range s.specs {
		if !used[spec.alias] {
			continue
		}

		// パッケージ名のまま参照し、import パスの末尾がパッケージ名のときは、名前を書かない
		if spec.alias != spec.name || !strings.HasSuffix(spec.path, spec.alias) {
			lines.WriteString(spec.alias + " ")
		}
		lines.WriteString(strconv.Quote(spec.path) + "\n")
	}

	return lines.String()
}

func (s *Imports) findByPath(path string) *importSpec {
	for _, spec := range s.specs {
		if spec.path == path {
			return spec
		}
	}

	return nil
}

func (s *Imports) findByAlias(alias string) *importSpec {
	for _, spec := range s.specs {
		if spec.alias == alias {
			return spec
		}
	}

	return nil
}

// trimVendor は、vendor ディレクトリの下のパッケージのパスから、/vendor/ までを除く。
func trimVendor(path string) string {
	if _, after, found := strings.CutLast(path, "/vendor/"); found {
		return after
	}

	return path
}
