package codegen

import (
	"errors"
	"fmt"
	gotypes "go/types"
	"maps"
	"slices"
	"strings"
	"unicode"

	gqlgenconfig "github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/Yamashou/gqlgenc/v3/config"

	graphql "github.com/vektah/gqlparser/v2/ast"
)

type GoTypeGenerator struct {
	cfg    *config.Config
	binder *gqlgenconfig.Binder
	types  map[string]gotypes.Type
	err    error
}

func NewGoTypeGenerator(cfg *config.Config) *GoTypeGenerator {
	return &GoTypeGenerator{
		cfg:    cfg,
		binder: cfg.GQLGenConfig.NewBinder(),
		types:  map[string]gotypes.Type{},
	}
}

func (g *GoTypeGenerator) CreateGoTypes(operations graphql.OperationList) ([]gotypes.Type, error) {
	for _, operation := range operations {
		t := g.newFields(operation.Name, operation.SelectionSet).goStructType()
		g.newGoNamedType(operation.Name, false, t)
	}

	if g.err != nil {
		return nil, g.err
	}

	return g.goTypes(), nil
}

func (g *GoTypeGenerator) goTypes() []gotypes.Type {
	return slices.SortedFunc(maps.Values(g.types), func(a, b gotypes.Type) int {
		return strings.Compare(strings.TrimPrefix(a.String(), "*"), strings.TrimPrefix(b.String(), "*"))
	})
}

// setErr は最初に起きたエラーだけを保持する (first-error-wins)。型生成は途中で止めず走査後に
// g.err をまとめて返すため、後続のエラーで最初の原因が上書きされないようにする。
func (g *GoTypeGenerator) setErr(err error) {
	if err != nil && g.err == nil {
		g.err = err
	}
}

// When parentTypeName is empty, the parent is an inline fragment
func (g *GoTypeGenerator) newFields(parentTypeName string, selectionSet graphql.SelectionSet) Fields {
	fields := make(Fields, 0, len(selectionSet))
	for _, selection := range selectionSet {
		fields = append(fields, g.newField(parentTypeName, selection))
	}

	g.setErr(fields.checkGoNameCollision(parentTypeName))

	return fields
}

// When parentTypeName is empty, the parent is an inline fragment
func (g *GoTypeGenerator) newField(parentTypeName string, selection graphql.Selection) *Field {
	switch sel := selection.(type) {
	case *graphql.Field:
		tags := []string{fmt.Sprintf(`json:"%s"`, sel.Alias)}
		// @goFragment(type:) でフィールド選択を既存 Go 型にバインドする場合は、
		// 選択セットを再帰せずバインド型をそのまま使う。
		if baseType, _, ok := g.boundGoType(sel.Directives); ok {
			t := g.newObjectGoType(baseType, sel.Definition.Type)
			return newField(Scalar, t, sel.Alias, tags)
		}
		typeKind, t := g.newTypeKindAndGoType(parentTypeName, sel)
		return newField(typeKind, t, sel.Alias, tags)
	case *graphql.FragmentSpread:
		// @goFragment(type:) または autobind でフラグメントを既存 Go 型にバインドする
		// 場合は、型生成せずバインド型を埋め込む。埋め込みフィールド名はバインド型名。
		if baseType, name, ok := g.fragmentBinding(sel.Definition); ok {
			return newField(FragmentSpread, baseType, name, []string{jsonIgnoreTag})
		}
		structType := g.newFields(sel.Name, sel.Definition.SelectionSet).goStructType()
		namedType := g.newGoNamedType(sel.Name, true, structType)
		return newField(FragmentSpread, namedType, sel.Name, []string{jsonIgnoreTag})
	case *graphql.InlineFragment:
		structType := g.newFields("", sel.SelectionSet).goStructType()
		pointerType := gotypes.NewPointer(structType)
		tags := []string{jsonIgnoreTag}
		return newField(InlineFragment, pointerType, sel.TypeCondition, tags)
	}
	// 事前条件: selection は *Field/*FragmentSpread/*InlineFragment のいずれか。
	panic("unexpected selection type")
}

func (g *GoTypeGenerator) newTypeKindAndGoType(parentTypeName string, sel *graphql.Field) (TypeKind, gotypes.Type) {
	typeName := fieldTypeName(parentTypeName, sel.Alias, g.cfg.GQLGencConfig.ExportQueryType)
	fields := g.newFields(typeName, sel.SelectionSet)
	if len(fields) == 0 {
		t := g.newScalarGoType(sel.Definition.Type)
		return Scalar, t
	}

	// Create base type (always non-null) then wrap with list structure
	baseType := g.newGoNamedType(typeName, true, fields.goStructType())
	t := g.newObjectGoType(baseType, sel.Definition.Type)
	return Object, t
}

// newScalarGoType recursively builds Go type from GraphQL scalar type structure
func (g *GoTypeGenerator) newScalarGoType(gqlType *graphql.Type) gotypes.Type {
	// Base case: named type (e.g., String, Int, Status)
	if gqlType.NamedType != "" {
		return g.findGoType(gqlType.NamedType, gqlType.NonNull)
	}

	// List type case
	if gqlType.Elem != nil {
		elemType := g.newScalarGoType(gqlType.Elem)
		sliceType := gotypes.NewSlice(elemType)
		if !gqlType.NonNull {
			return gotypes.NewPointer(sliceType)
		}
		return sliceType
	}

	// 事前条件: gqlType は NamedType かリスト(Elem)を持つ。
	panic(fmt.Sprintf("unexpected GraphQL type structure: %+v", gqlType))
}

// newObjectGoType wraps object base type with GraphQL type structure (elements are always pointers)
func (g *GoTypeGenerator) newObjectGoType(baseType gotypes.Type, gqlType *graphql.Type) gotypes.Type {
	// Base case: named type
	if gqlType.NamedType != "" {
		if !gqlType.NonNull {
			return gotypes.NewPointer(baseType)
		}
		return baseType
	}

	// List type case: elements are always pointers for object types
	if gqlType.Elem != nil {
		elemBaseType := gotypes.NewPointer(baseType)
		elemType := g.newObjectGoType(elemBaseType, gqlType.Elem)
		sliceType := gotypes.NewSlice(elemType)
		if !gqlType.NonNull {
			return gotypes.NewPointer(sliceType)
		}
		return sliceType
	}

	// 事前条件: gqlType は NamedType かリスト(Elem)を持つ。
	panic(fmt.Sprintf("unexpected GraphQL type structure: %+v", gqlType))
}

func (g *GoTypeGenerator) newGoNamedType(typeName string, nonnull bool, t gotypes.Type) gotypes.Type {
	var namedType gotypes.Type
	namedType = gotypes.NewNamed(gotypes.NewTypeName(0, g.cfg.GQLGencConfig.QueryGen.Pkg(), typeName, nil), t, nil)
	if !nonnull {
		namedType = gotypes.NewPointer(namedType)
	}
	// new type set to g.types
	g.types[namedType.String()] = namedType
	return namedType
}

// The typeName passed to the Type argument must be the type name derived from the analysis result, such as from selections
func (g *GoTypeGenerator) findGoType(typeName string, nonNull bool) gotypes.Type {
	t, err := resolveModelType(g.binder, g.cfg.GQLGenConfig.Models, typeName, nonNull)
	g.setErr(err)
	return t
}

// resolveModelType は GraphQL 型名に束縛された Go 型を gqlgen の model マップから解決する。
// nullable な場合はポインタで包む。codegen 内の型解決はこの関数に集約する。
func resolveModelType(binder *gqlgenconfig.Binder, models gqlgenconfig.TypeMap, typeName string, nonNull bool) (gotypes.Type, error) {
	bindings := models[typeName].Model
	if len(bindings) == 0 {
		// UC1 (gqlgen.model 未指定) で enum / input が autobind / @goModel のいずれにも
		// 束縛されていないと到達する。panic ではなく原因と対処を示すエラーで失敗させる。
		return gotypes.Typ[gotypes.Invalid], fmt.Errorf("no Go model is bound for GraphQL type %q: generate it by setting gqlgen.model, or bind it via autobind / @goModel", typeName)
	}

	goType, err := binder.FindTypeFromName(bindings[0])
	if err != nil {
		return gotypes.Typ[gotypes.Invalid], fmt.Errorf("resolve Go model for GraphQL type %q: %w", typeName, err)
	}
	if !nonNull {
		goType = gotypes.NewPointer(goType)
	}

	return goType, nil
}

// boundGoType は @goFragment(type:) ディレクティブがあればバインド先の Go 型と
// その非修飾名を返す。フラグメントやフィールド選択を既存 Go 型にバインドするために使う。
//
// 戻り値:
//   - gotypes.Type: バインド先の Go 型（要素型。リスト/NonNull の包みは呼び出し側で行う）
//   - string: バインド先型の非修飾名（埋め込みフィールド名に使う）
//   - bool: @goFragment が付いている場合は true
func (g *GoTypeGenerator) boundGoType(directives graphql.DirectiveList) (gotypes.Type, string, bool) {
	directive := directives.ForName(goFragmentDirectiveName)
	if directive == nil {
		return nil, "", false
	}

	arg := directive.Arguments.ForName(goFragmentArgType)
	if arg == nil {
		return nil, "", false
	}

	value, err := arg.Value.Value(nil)
	if err != nil {
		g.setErr(fmt.Errorf("@goFragment: %w", err))
		return nil, "", false
	}

	typeName, ok := value.(string)
	if !ok {
		g.setErr(errors.New("@goFragment: type argument must be a string"))
		return nil, "", false
	}

	goType, err := g.binder.FindTypeFromName(typeName)
	if err != nil {
		g.setErr(fmt.Errorf("@goFragment: failed to resolve %q: %w", typeName, err))
		return nil, "", false
	}

	return goType, goTypeName(goType), true
}

// autobindFragment は gqlgen の autobind のクエリ版。gqlgenc.autobind に指定した
// パッケージに fragment 名と同名の型があれば、その Go 型とその非修飾名を返す。
// @goFragment(type:) と違い、クエリに書かず YAML 設定だけでフラグメントを既存 Go 型に
// バインドできる。戻り値は boundGoType と同じ（バインド先 Go 型、埋め込みフィールド名、
// 見つかったか）。
func (g *GoTypeGenerator) autobindFragment(fragmentName string) (gotypes.Type, string, bool) {
	for _, pkgPath := range g.cfg.GQLGencConfig.Autobind {
		goType, err := g.binder.FindType(pkgPath, fragmentName)
		if err != nil {
			if errors.Is(err, gqlgenconfig.ErrTypeNotFound) {
				continue
			}
			g.setErr(fmt.Errorf("autobind: failed to load package %q: %w", pkgPath, err))
			return nil, "", false
		}

		return goType, goTypeName(goType), true
	}

	return nil, "", false
}

// fragmentBinding はフラグメントスプレッドをバインドする既存 Go 型を返す。
// 明示的な @goFragment(type:) ディレクティブを優先し、無ければ autobind による
// 名前一致を試す。どちらにも該当しなければ ok=false を返し、呼び出し側で型を生成する。
func (g *GoTypeGenerator) fragmentBinding(def *graphql.FragmentDefinition) (gotypes.Type, string, bool) {
	if baseType, name, ok := g.boundGoType(def.Directives); ok {
		return baseType, name, true
	}

	return g.autobindFragment(def.Name)
}

func fieldTypeName(parentTypeName, fieldName string, exportQueryType bool) string {
	if exportQueryType {
		return fmt.Sprintf("%s_%s", firstUpper(parentTypeName), templates.ToGo(fieldName))
	}

	// default: query type is not exported
	return fmt.Sprintf("%s_%s", firstLower(parentTypeName), templates.ToGo(fieldName))
}

func firstUpper(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func firstLower(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

//////////////////////////////////////////////////////////////////////////////////////////////////
// Field

type TypeKind string

const (
	Scalar         TypeKind = "Scalar"
	Object         TypeKind = "Object"
	FragmentSpread TypeKind = "FragmentSpread"
	InlineFragment TypeKind = "InlineFragment"
)

const jsonIgnoreTag = `json:"-"`

type Field struct {
	Name     string
	Type     gotypes.Type
	Tags     []string
	TypeKind TypeKind
}

func newField(typeKind TypeKind, fieldType gotypes.Type, name string, tags []string) *Field {
	return &Field{
		Name:     name,
		Type:     fieldType,
		Tags:     tags,
		TypeKind: typeKind,
	}
}

func (r *Field) goVar() *gotypes.Var {
	return gotypes.NewField(0, nil, templates.ToGo(r.Name), r.Type, r.TypeKind == FragmentSpread)
}

func (r *Field) joinTags() string {
	return strings.Join(r.Tags, " ")
}

type Fields []*Field

func (fs Fields) goStructType() *gotypes.Struct {
	// Go struct fields do not allow fields with the same name, so we remove duplicates
	fields := fs.uniqueByName()
	vars := make([]*gotypes.Var, 0, len(fields))
	for _, field := range fields {
		vars = append(vars, field.goVar())
	}
	tags := make([]string, 0, len(fields))
	for _, field := range fields {
		tags = append(tags, field.joinTags())
	}
	return gotypes.NewStruct(vars, tags)
}

func (fs Fields) uniqueByName() Fields {
	// 異なるセレクション名でも Go フィールド名が衝突し得るため、Go 名でまとめて
	// go/types.NewStruct の panic を防ぐ (衝突は checkGoNameCollision がエラーにする)
	fieldMapByName := make(map[string]*Field, len(fs))
	for _, field := range fs {
		fieldMapByName[templates.ToGo(field.Name)] = field
	}
	return slices.SortedFunc(maps.Values(fieldMapByName), func(a *Field, b *Field) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// checkGoNameCollision は、異なるセレクション名が同じ Go フィールド名に変換される
// 衝突を検出する (例: foo_bar と fooBar はどちらも FooBar になる)。
// 衝突はクエリ側で alias を付けることでしか解消できないため、エラーとして報告する。
func (fs Fields) checkGoNameCollision(parentTypeName string) error {
	if parentTypeName == "" {
		parentTypeName = "inline fragment"
	}

	rawNamesByGoName := make(map[string][]string, len(fs))
	for _, field := range fs {
		goName := templates.ToGo(field.Name)
		if !slices.Contains(rawNamesByGoName[goName], field.Name) {
			rawNamesByGoName[goName] = append(rawNamesByGoName[goName], field.Name)
		}
	}

	for _, goName := range slices.Sorted(maps.Keys(rawNamesByGoName)) {
		rawNames := rawNamesByGoName[goName]
		if len(rawNames) < 2 {
			continue
		}
		slices.Sort(rawNames)
		return fmt.Errorf("fields %s in %s map to the same Go field name %q: add an alias to one of them in the query", strings.Join(rawNames, " and "), parentTypeName, goName)
	}

	return nil
}

// goFragmentDirectiveName / goFragmentArgType は @goFragment(type:) の名前。
const (
	goFragmentDirectiveName = "goFragment"
	goFragmentArgType       = "type"
)

// goTypeName はバインド先 Go 型の非修飾名を返す（埋め込みフィールド名に使う）。
func goTypeName(t gotypes.Type) string {
	if named, ok := t.(*gotypes.Named); ok {
		return named.Obj().Name()
	}
	if pointer, ok := t.(*gotypes.Pointer); ok {
		return goTypeName(pointer.Elem())
	}
	return ""
}
