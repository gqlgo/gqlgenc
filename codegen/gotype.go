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
	"github.com/Yamashou/gqlgenc/v3/internal/typebind"

	graphql "github.com/vektah/gqlparser/v2/ast"
)

// ResponseType は、query_gen.go に宣言するレスポンスの型 1 つを表す。オペレーション・フィールドの
// 選択セット・フラグメント・インラインフラグメントの枝ごとに 1 つ作る。
type ResponseType struct {
	// Name は Go の型名
	Name string
	// Fields は宣言するフィールド。Go のフィールド名ごとに 1 つを、選択の名前の順に並べる
	Fields Fields
}

type GoTypeGenerator struct {
	cfg    *config.Config
	schema *graphql.Schema
	// models は、スキーマの型ごとの束縛先の Go 型
	models gqlgenconfig.TypeMap
	binder *typebind.Binder
	// types は、生成する型を Go の型名で引く表
	types map[string]*ResponseType
	err   error
	// typeNameOrigins は、生成する型の名前ごとに、その型を生成する選択の説明を持つ (claimTypeName)
	typeNameOrigins map[string]string
	// typeNameErr は、別々の選択が同じ型の名前を使った最初の衝突。err の後に報告する
	typeNameErr error
}

// NewGoTypeGenerator は GoTypeGenerator を作る。schema は読み込んだスキーマ、models はスキーマの型の
// 束縛、binder は束縛先の Go 型を解決する binder。
func NewGoTypeGenerator(cfg *config.Config, schema *graphql.Schema, models gqlgenconfig.TypeMap, binder *typebind.Binder) *GoTypeGenerator {
	return &GoTypeGenerator{
		cfg:             cfg,
		schema:          schema,
		models:          models,
		binder:          binder,
		types:           map[string]*ResponseType{},
		typeNameOrigins: map[string]string{},
		typeNameErr:     nil,
	}
}

// CreateResponseTypes は、オペレーションのレスポンスを表す型を、選択セットの入れ子の型も含めてすべて作り、
// 型名の順に返す。
func (g *GoTypeGenerator) CreateResponseTypes(operations graphql.OperationList) ([]*ResponseType, error) {
	for _, operation := range operations {
		// client_gen.go と query_gen.go の Document 定数は、オペレーション名を Go の名前
		// (templates.ToGo: getUser → GetUser、GetUserId → GetUserID) にして参照する。
		// 型も同じ名前で宣言しないと、Go の名前の形でないオペレーション名で参照先が無くなる。
		typeName := templates.ToGo(operation.Name)
		fields := g.newFields(typeName, operation.SelectionSet)
		g.claimTypeName(typeName, "operation "+operation.Name)
		g.newResponseType(typeName, fields)
	}

	if g.err != nil {
		return nil, g.err
	}

	// 型の名前の衝突は、ほかのエラーの後に報告する。Go のフィールド名の衝突 (foo_bar と fooBar) は
	// 型の名前も重ねる (どちらも <親の型>_FooBar) ため、直し方を示すフィールド名の衝突のエラーを先に返す。
	if g.typeNameErr != nil {
		return nil, g.typeNameErr
	}

	return slices.SortedFunc(maps.Values(g.types), func(a, b *ResponseType) int {
		return strings.Compare(a.Name, b.Name)
	}), nil
}

// setErr は最初に起きたエラーだけを保持する (first-error-wins)。型生成は途中で止めず走査後に
// g.err をまとめて返すため、後続のエラーで最初の原因が上書きされないようにする。
func (g *GoTypeGenerator) setErr(err error) {
	if err != nil && g.err == nil {
		g.err = err
	}
}

func (g *GoTypeGenerator) newFields(parentTypeName string, selectionSet graphql.SelectionSet) Fields {
	selectionSet = mergeSelections(selectionSet)

	fields := make(Fields, 0, len(selectionSet))
	for _, selection := range selectionSet {
		fields = append(fields, g.newField(parentTypeName, selection))
	}

	g.setErr(fields.checkGoNameCollision(parentTypeName))

	return fields
}

// mergeSelections は、同じ選択セットに複数回現れる選択を GraphQL のフィールド収集の規則どおり
// 1 つにまとめる。まとめないと同じ名前の Go 型を 2 回作ることになり、後の選択だけが残って
// 先の選択のサブフィールドが生成型から落ちる (gqlgo/gqlgenc#282)。
//
//   - 同じレスポンスキー (alias) のフィールド: 最初の選択の位置に、すべての選択をまとめる (mergeFields)
//   - 同じ型へのインラインフラグメント: 最初の選択に、後の選択のセレクションを足す。型条件の無い
//     もの (`... { id }`・`... @include(if: $x) { id }`) は、外側の型への条件として扱う
//   - 同じフラグメントのスプレッド: 最初の選択だけを残す
//
// 事前条件: selectionSet は検証済み (インラインフラグメントの ObjectDefinition が束縛されている)。
// AST はサーバーへ送る Document の生成にも使うため書き換えず、まとめた選択は写しで返す。
func mergeSelections(selectionSet graphql.SelectionSet) graphql.SelectionSet {
	// 同じレスポンスキーのフィールドの選択を、現れた順に集める。条件 (@skip / @include) の扱いは
	// 同じキーのすべての選択を見て決めるため、2 つずつではなくまとめて渡す
	fieldsByAlias := make(map[string][]*graphql.Field, len(selectionSet))
	for _, selection := range selectionSet {
		if field, ok := selection.(*graphql.Field); ok {
			fieldsByAlias[field.Alias] = append(fieldsByAlias[field.Alias], field)
		}
	}

	merged := make(graphql.SelectionSet, 0, len(selectionSet))
	// 最初に現れた位置へまとめるため、選択の種類とキーごとに merged の位置を覚える
	positions := make(map[string]int, len(selectionSet))
	inlineFragments := make(map[string]*graphql.InlineFragment)

	for _, selection := range selectionSet {
		switch sel := selection.(type) {
		case *graphql.Field:
			key := "field:" + sel.Alias
			if _, ok := positions[key]; ok {
				continue
			}
			positions[key] = len(merged)
			merged = append(merged, mergeFields(fieldsByAlias[sel.Alias]))
		case *graphql.InlineFragment:
			sel = inlineFragmentOnEnclosingType(sel)
			key := "inlineFragment:" + sel.TypeCondition
			if first, ok := inlineFragments[sel.TypeCondition]; ok {
				sel = mergeInlineFragment(first, sel)
				merged[positions[key]] = sel
			} else {
				positions[key] = len(merged)
				merged = append(merged, sel)
			}
			inlineFragments[sel.TypeCondition] = sel
		case *graphql.FragmentSpread:
			key := "fragmentSpread:" + sel.Name
			if _, ok := positions[key]; ok {
				continue
			}
			positions[key] = len(merged)
			merged = append(merged, sel)
		}
	}

	return merged
}

// mergeFields は、同じレスポンスキーの選択 (現れた順) を 1 つにまとめた写しを返す。選択が 1 つなら
// それをそのまま返す。サブセレクションはすべての選択のものを順に連ねる (次の階層の mergeSelections が
// さらにまとめる)。
//
// @skip / @include は次のように扱う。
//   - まとめたフィールド: 条件の無い選択が 1 つでもあれば条件を外す (サーバーは必ずそのフィールドを
//     返すため、非 null のフィールドをポインタにしない)。無ければ最初の選択の条件を残す (どの条件も
//     偽ならフィールドが返らないため、ポインタにする)
//   - サブセレクション: 選択ごとの条件が同じでなければ、条件付きの選択のサブセレクションにその選択の
//     条件を付ける。まとめたフィールドがレスポンスにあっても、条件が偽の選択のサブフィールドは
//     返らないため、条件の無い選択にも現れるもの以外をポインタにする
//
// 事前条件: fields は空でなく、どれも同じ Alias を持つ。
func mergeFields(fields []*graphql.Field) *graphql.Field {
	first := fields[0]
	if len(fields) == 1 {
		return first
	}

	sameConditions := true
	for _, field := range fields[1:] {
		if conditionKey(field.Directives) != conditionKey(first.Directives) {
			sameConditions = false
			break
		}
	}

	merged := *first
	merged.SelectionSet = nil
	unconditional := false
	for _, field := range fields {
		conditions := conditionalDirectives(field.Directives)
		if len(conditions) == 0 {
			unconditional = true
		}

		if sameConditions || len(conditions) == 0 {
			merged.SelectionSet = append(merged.SelectionSet, field.SelectionSet...)
			continue
		}
		merged.SelectionSet = append(merged.SelectionSet, withConditions(field.SelectionSet, conditions)...)
	}

	if unconditional {
		merged.Directives = withoutConditionalDirectives(first.Directives)
	}

	return &merged
}

// conditionalDirectives は @skip / @include だけを、書かれた順に返す。
func conditionalDirectives(directives graphql.DirectiveList) graphql.DirectiveList {
	var conditions graphql.DirectiveList
	for _, directive := range directives {
		if directive.Name == skipDirectiveName || directive.Name == includeDirectiveName {
			conditions = append(conditions, directive)
		}
	}

	return conditions
}

// conditionKey は、@skip / @include の名前と引数を 1 つの文字列にする。同じ条件の選択は同じ文字列になる。
func conditionKey(directives graphql.DirectiveList) string {
	var key strings.Builder
	for _, directive := range conditionalDirectives(directives) {
		key.WriteString("@" + directive.Name)
		for _, argument := range directive.Arguments {
			key.WriteString("(" + argument.Name + ":" + argument.Value.String() + ")")
		}
	}

	return key.String()
}

// withConditions は、選択セットの各フィールドに conditions (@skip / @include) を足した写しを返す。
// インラインフラグメントは、その中の選択に足す。フラグメントスプレッドはそのまま返す (フラグメントの
// 型はほかの選択と共有するため、条件を付けても型は変わらない)。
func withConditions(selectionSet graphql.SelectionSet, conditions graphql.DirectiveList) graphql.SelectionSet {
	conditioned := make(graphql.SelectionSet, 0, len(selectionSet))
	for _, selection := range selectionSet {
		switch sel := selection.(type) {
		case *graphql.Field:
			field := *sel
			field.Directives = slices.Concat(sel.Directives, conditions)
			conditioned = append(conditioned, &field)
		case *graphql.InlineFragment:
			fragment := *sel
			fragment.SelectionSet = withConditions(sel.SelectionSet, conditions)
			conditioned = append(conditioned, &fragment)
		default:
			conditioned = append(conditioned, selection)
		}
	}

	return conditioned
}

// withoutConditionalDirectives は @skip / @include を除いたディレクティブの写しを返す。
func withoutConditionalDirectives(directives graphql.DirectiveList) graphql.DirectiveList {
	kept := make(graphql.DirectiveList, 0, len(directives))
	for _, directive := range directives {
		if directive.Name != skipDirectiveName && directive.Name != includeDirectiveName {
			kept = append(kept, directive)
		}
	}

	return kept
}

// mergeInlineFragment は、同じ型への 2 つのインラインフラグメントを 1 つにまとめた写しを返す。
func mergeInlineFragment(first, later *graphql.InlineFragment) *graphql.InlineFragment {
	merged := *first
	merged.SelectionSet = slices.Concat(first.SelectionSet, later.SelectionSet)

	return &merged
}

// inlineFragmentOnEnclosingType は、型条件の無いインラインフラグメントを外側の型への条件にした
// 写しを返す。型条件があるものはそのまま返す。型条件が無いままだと、空の名前のフィールドと型を
// 作ってコンパイルできないコードになる。
func inlineFragmentOnEnclosingType(sel *graphql.InlineFragment) *graphql.InlineFragment {
	if sel.TypeCondition != "" || sel.ObjectDefinition == nil {
		return sel
	}

	resolved := *sel
	resolved.TypeCondition = sel.ObjectDefinition.Name

	return &resolved
}

func (g *GoTypeGenerator) newField(parentTypeName string, selection graphql.Selection) *Field {
	switch sel := selection.(type) {
	case *graphql.Field:
		// @goFragment(type:) でフィールド選択を既存 Go 型にバインドする場合は、
		// 選択セットを再帰せずバインド型をそのまま使う。
		var typeKind TypeKind
		var t gotypes.Type
		if baseType, _, ok := g.boundGoType(sel.Directives); ok {
			typeKind, t = Scalar, g.newObjectGoType(baseType, sel.Definition.Type)
		} else {
			typeKind, t = g.newTypeKindAndGoType(parentTypeName, sel)
		}
		// @skip / @include 付きのフィールドはレスポンスから条件付きで欠落し得るため、
		// 非null型でもポインタにして「欠落 = nil」を表現できるようにする。
		// __typename は除く。querygen が string として switch で分岐するためで、ポインタにすると
		// コンパイルできない。条件付きの選択をまとめると、gqlparser がその選択に注入した __typename
		// にも条件が付く (mergeFields)。欠落したときは空文字列になり、どの枝にも一致しない。
		if hasConditionalDirective(sel.Directives) && sel.Name != typenameFieldName {
			t = pointerize(t)
		}
		return &Field{Name: sel.Alias, Type: t, TypeKind: typeKind, PossibleTypes: nil}
	case *graphql.FragmentSpread:
		// @goFragment(type:) または autobind でフラグメントを既存 Go 型にバインドする
		// 場合は、型生成せずバインド型をそのまま使う。フィールド名はバインド型名。
		if baseType, name, ok := g.fragmentBinding(sel.Definition); ok {
			return &Field{Name: name, Type: baseType, TypeKind: FragmentSpread, PossibleTypes: nil}
		}
		fields := g.newFields(sel.Name, sel.Definition.SelectionSet)
		g.claimTypeName(sel.Name, "fragment "+sel.Name)
		namedType := g.newResponseType(sel.Name, fields)
		return &Field{Name: sel.Name, Type: namedType, TypeKind: FragmentSpread, PossibleTypes: nil}
	case *graphql.InlineFragment:
		// インラインフラグメントの枝も名前付き型で生成する。匿名構造体だと利用側が値を
		// 構築するとき Go の型同一性ルールにより json タグまで一致するリテラルを書く必要が
		// あり、テストや変換コードが書けないため。型名は 親型名_型条件 (例:
		// UserOperation_User_User) とする。
		typeName := fieldTypeName(parentTypeName, sel.TypeCondition)
		fields := g.newFields(typeName, sel.SelectionSet)
		g.claimTypeName(typeName, fmt.Sprintf("inline fragment on %s in %s", sel.TypeCondition, parentTypeName))
		namedType := g.newResponseType(typeName, fields)
		// 枝に値が入るのは、レスポンスの __typename が型条件を満たす具体型のとき。interface / union への
		// 型条件は __typename と名前が一致しないため、満たす具体型を querygen へ渡す。
		return &Field{
			Name:          sel.TypeCondition,
			Type:          gotypes.NewPointer(namedType),
			TypeKind:      InlineFragment,
			PossibleTypes: g.possibleTypeNames(sel.TypeCondition, sel.ObjectDefinition),
		}
	}
	// 事前条件: selection は *Field/*FragmentSpread/*InlineFragment のいずれか。
	panic("unexpected selection type")
}

// possibleTypeNames は、型条件を満たす具体型 (レスポンスの __typename になり得る Object 型) の名前を
// 名前順で返す。Object 型はそれ自身、interface は実装する Object 型、union はメンバーになる。
// interface を実装する interface は __typename にならないため含めない。スキーマに無い型は nil を返す。
//
// enclosing は、インラインフラグメントを書いた位置の型 (nil なら絞り込まない)。その位置の値に
// なり得ない具体型は除く。例えば File を返すフィールドの中の `... on Node` は、Node を実装していても
// File を実装していない型にはならない。
func (g *GoTypeGenerator) possibleTypeNames(typeCondition string, enclosing *graphql.Definition) []string {
	schema := g.schema
	definition, ok := schema.Types[typeCondition]
	if !ok {
		return nil
	}

	// 定義のポインタではなく名前で比べる (enclosing がスキーマの定義と同じ実体とは限らない)
	var enclosingTypeNames map[string]struct{}
	if enclosing != nil {
		enclosingTypes := schema.GetPossibleTypes(enclosing)
		enclosingTypeNames = make(map[string]struct{}, len(enclosingTypes))
		for _, enclosingType := range enclosingTypes {
			enclosingTypeNames[enclosingType.Name] = struct{}{}
		}
	}

	possibleTypes := schema.GetPossibleTypes(definition)
	typeNames := make([]string, 0, len(possibleTypes))
	for _, possibleType := range possibleTypes {
		if possibleType.Kind != graphql.Object {
			continue
		}
		if _, ok := enclosingTypeNames[possibleType.Name]; enclosing != nil && !ok {
			continue
		}
		typeNames = append(typeNames, possibleType.Name)
	}
	slices.Sort(typeNames)

	return typeNames
}

const (
	skipDirectiveName    = "skip"
	includeDirectiveName = "include"
)

// hasConditionalDirective は @skip / @include が付いているかを返す。これらはフィールドを
// レスポンスから条件付きで欠落させ得る。
func hasConditionalDirective(directives graphql.DirectiveList) bool {
	return directives.ForName(skipDirectiveName) != nil || directives.ForName(includeDirectiveName) != nil
}

// pointerize は型がポインタでなければポインタで包む (既にポインタならそのまま返す)。
func pointerize(t gotypes.Type) gotypes.Type {
	if _, ok := t.(*gotypes.Pointer); ok {
		return t
	}
	return gotypes.NewPointer(t)
}

func (g *GoTypeGenerator) newTypeKindAndGoType(parentTypeName string, sel *graphql.Field) (TypeKind, gotypes.Type) {
	typeName := fieldTypeName(parentTypeName, sel.Alias)
	fields := g.newFields(typeName, sel.SelectionSet)
	if len(fields) == 0 {
		fieldType := sel.Definition.Type
		if sel.Name == typenameFieldName {
			fieldType = &graphql.Type{NamedType: fieldType.NamedType, NonNull: true}
		}
		t := scalarGoType(g.binder, g.models, fieldType, g.setErr)
		return Scalar, t
	}

	// Create base type (always non-null) then wrap with list structure
	g.claimTypeName(typeName, fmt.Sprintf("field %s in %s", sel.Alias, parentTypeName))
	baseType := g.newResponseType(typeName, fields)
	t := g.newObjectGoType(baseType, sel.Definition.Type)
	return Object, t
}

// scalarGoType は GraphQL のスカラー的な型 (レスポンスのリーフ scalar/enum、変数の Input 型)
// を、リスト構造を保ったまま Go 型へ変換する。リストは要素型のスライスにし、nullable なリストは
// ポインタで包む。最内の名前付き型はモデルマップから解決し、最初の解決エラーを setErr に渡す。
// GoTypeGenerator (リーフ) と OperationGenerator (変数) の双方が使う共有関数。
func scalarGoType(binder *typebind.Binder, models gqlgenconfig.TypeMap, t *graphql.Type, setErr func(error)) gotypes.Type {
	// Base case: named type (e.g., String, Int, Status)
	if t.NamedType != "" {
		goType, err := resolveModelType(binder, models, t.NamedType, t.NonNull)
		setErr(err)
		return goType
	}

	// List type case
	if t.Elem != nil {
		sliceType := gotypes.NewSlice(scalarGoType(binder, models, t.Elem, setErr))
		if !t.NonNull {
			return gotypes.NewPointer(sliceType)
		}
		return sliceType
	}

	// 事前条件: t は NamedType かリスト(Elem)を持つ。
	panic(fmt.Sprintf("unexpected GraphQL type structure: %+v", t))
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

	// List type case: object elements are always a single pointer. The element
	// pointer must be added once, at the innermost level, not once per list
	// level (else [[Cell!]!] would become [][]**Cell instead of [][]*Cell).
	if gqlType.Elem != nil {
		var elemType gotypes.Type
		if gqlType.Elem.NamedType != "" {
			elemType = gotypes.NewPointer(baseType)
		} else {
			elemType = g.newObjectGoType(baseType, gqlType.Elem)
		}
		sliceType := gotypes.NewSlice(elemType)
		if !gqlType.NonNull {
			return gotypes.NewPointer(sliceType)
		}
		return sliceType
	}

	// 事前条件: gqlType は NamedType かリスト(Elem)を持つ。
	panic(fmt.Sprintf("unexpected GraphQL type structure: %+v", gqlType))
}

// newResponseType は、名前 typeName とフィールド fields の型を、生成する型として記録する。
// 戻り値は、ほかの型のフィールドからこの型を参照するための go/types の型。参照の表記
// (パッケージの修飾・ポインタ・スライス) にだけ使うため、underlying は空の struct にしている。
// フィールドは ResponseType が持つ。
func (g *GoTypeGenerator) newResponseType(typeName string, fields Fields) gotypes.Type {
	g.types[typeName] = &ResponseType{Name: typeName, Fields: fields.uniqueByName()}

	return gotypes.NewNamed(gotypes.NewTypeName(0, g.cfg.QueryGen.Pkg(), typeName, nil), gotypes.NewStruct(nil, nil), nil)
}

// claimTypeName は、生成する型の名前 typeName を、origin (その型を生成する選択の説明) が使うものとして
// 記録する。別の origin が同じ名前を使っていれば、最初の衝突を typeNameErr に残す。衝突を許すと、同じ
// 名前の型を 2 回宣言してコンパイルできないコードになるか (オペレーションとフラグメントの型)、後の型が
// 先の型を上書きして先の選択のフィールドが生成型から落ちる (g.types のキーが同じになる型)。
// 同じフラグメントを複数回展開したときのように、同じ origin が同じ名前を使うのは衝突ではない。
func (g *GoTypeGenerator) claimTypeName(typeName, origin string) {
	existing, ok := g.typeNameOrigins[typeName]
	if !ok {
		g.typeNameOrigins[typeName] = origin
		return
	}

	if existing != origin && g.typeNameErr == nil {
		g.typeNameErr = fmt.Errorf("%s and %s generate the same Go type name %q: rename one of them in the query", existing, origin, typeName)
	}
}

// resolveModelType は GraphQL 型名に束縛された Go 型を gqlgen の model マップから解決する。
// nullable な場合はポインタで包む。codegen 内の型解決はこの関数に集約する。
func resolveModelType(binder *typebind.Binder, models gqlgenconfig.TypeMap, typeName string, nonNull bool) (gotypes.Type, error) {
	bindings := models[typeName].Model
	if len(bindings) == 0 {
		// UC1 (generate.model.file 未指定) で enum / input が bind.type.packages / @goModel の
		// いずれにも束縛されていないと到達する。panic ではなく原因と対処を示すエラーで失敗させる。
		err := fmt.Errorf("no Go model is bound for GraphQL type %q: generate it by setting generate.model.file, or bind it via bind.type.packages / @goModel", typeName)
		// 束縛先のパッケージを読み込めていないと、そのパッケージの型はどれも束縛されずにここへ届く。
		// 束縛先に型が無いという設定の不備と見分けられるよう、読み込みのエラーを添える。
		if loadErr := binder.LoadErrors(); loadErr != nil {
			err = fmt.Errorf("%w (%w)", err, loadErr)
		}

		return gotypes.Typ[gotypes.Invalid], err
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
//   - string: バインド先型の非修飾名（フィールド名に使う）
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

// autobindFragment は gqlgen の autobind のクエリ版。bind.fragment.packages に指定した
// パッケージに fragment 名と同名の型があれば、その Go 型とその非修飾名を返す。
// @goFragment(type:) と違い、クエリに書かず YAML 設定だけでフラグメントを既存 Go 型に
// バインドできる。戻り値は boundGoType と同じ（バインド先 Go 型、フィールド名、
// 見つかったか）。
func (g *GoTypeGenerator) autobindFragment(fragmentName string) (gotypes.Type, string, bool) {
	for _, pkgPath := range g.cfg.FragmentAutobind {
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

// fieldTypeName builds the generated subtype name. Query response types are
// always exported, so the parent name is upper-cased.
func fieldTypeName(parentTypeName, fieldName string) string {
	return fmt.Sprintf("%s_%s", firstUpper(parentTypeName), templates.ToGo(fieldName))
}

func firstUpper(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
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

// typenameFieldName は interface / union を判別する GraphQL メタフィールド。gqlparser は注入された
// __typename を nullable な String で束縛するが、__typename は常に存在する非 null なので string で生成する。
const typenameFieldName = "__typename"

// Field は、レスポンスの型のフィールド 1 つを表す。選択セットの選択 1 つにあたる。
type Field struct {
	// Name は選択の名前。フィールドは別名 (別名が無ければフィールド名)、フラグメントスプレッドは
	// フラグメント名 (既存の Go 型に束縛したときはその型名)、インラインフラグメントは型条件の型名
	Name string
	// Type は Go のフィールドの型
	Type gotypes.Type
	// TypeKind は選択の種類
	TypeKind TypeKind
	// PossibleTypes は、インラインフラグメントの型条件を満たす具体型 (レスポンスの __typename に
	// なり得る Object 型) の名前。名前順に並べる。インラインフラグメントでなければ nil
	PossibleTypes []string
}

// GoName は Go のフィールド名を返す。
//
// フラグメントも名前付きのフィールドにし、埋め込まない。埋め込むと Go のフィールド昇格で
// 複数のフラグメントの同名のフィールドが曖昧になり (ambiguous selector)、利用者が t.Field を
// 読めなくなるため。アクセスは常に t.<FragmentName>.Field となる。
func (r *Field) GoName() string {
	return templates.ToGo(r.Name)
}

// IsFragment は、フラグメントスプレッドかインラインフラグメントかを返す。フラグメントのフィールドは、
// レスポンスの JSON に対応するメンバーを持たず、親と同じ JSON オブジェクトからデコードする。
func (r *Field) IsFragment() bool {
	return r.TypeKind == FragmentSpread || r.TypeKind == InlineFragment
}

type Fields []*Field

func (fs Fields) uniqueByName() Fields {
	// 異なるセレクション名でも Go フィールド名が衝突し得るため、Go 名ごとに 1 つにまとめる
	// (Go の struct は同じ名前のフィールドを持てない。衝突は checkGoNameCollision がエラーにする)
	fieldMapByName := make(map[string]*Field, len(fs))
	for _, field := range fs {
		fieldMapByName[field.GoName()] = field
	}
	return slices.SortedFunc(maps.Values(fieldMapByName), func(a *Field, b *Field) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// checkGoNameCollision は、異なる選択が同じ Go フィールド名に変換される衝突を検出する
// (例: foo_bar と fooBar はどちらも FooBar になる)。選択の種類 (フィールド・インラインフラグメント・
// フラグメントスプレッド) が違えば、名前が同じでも異なる選択として扱う (例: 別名 Item のフィールドと
// `... on Item` はどちらも Item になる)。uniqueByName は Go 名ごとに 1 つしか残さないため、衝突を
// 許すと選択が生成型から落ちる。衝突はクエリ側で別名を付けるかフラグメントの名前を変えることでしか
// 解消できないため、エラーとして報告する。
func (fs Fields) checkGoNameCollision(parentTypeName string) error {
	selectionsByGoName := make(map[string][]string, len(fs))
	for _, field := range fs {
		goName := templates.ToGo(field.Name)
		selection := field.selectionLabel()
		if !slices.Contains(selectionsByGoName[goName], selection) {
			selectionsByGoName[goName] = append(selectionsByGoName[goName], selection)
		}
	}

	for _, goName := range slices.Sorted(maps.Keys(selectionsByGoName)) {
		selections := selectionsByGoName[goName]
		if len(selections) < 2 {
			continue
		}
		slices.Sort(selections)
		return fmt.Errorf("%s in %s map to the same Go field name %q: add an alias to a field or rename a fragment in the query", strings.Join(selections, " and "), parentTypeName, goName)
	}

	return nil
}

// selectionLabel は、エラーメッセージで選択を示す文字列を返す (例: field foo_bar・inline fragment on Item)。
func (r *Field) selectionLabel() string {
	switch r.TypeKind {
	case FragmentSpread:
		return "fragment spread " + r.Name
	case InlineFragment:
		return "inline fragment on " + r.Name
	default:
		return "field " + r.Name
	}
}

// goFragmentDirectiveName / goFragmentArgType は @goFragment(type:) の名前。
const (
	goFragmentDirectiveName = "goFragment"
	goFragmentArgType       = "type"
)

// goTypeName はバインド先 Go 型の非修飾名を返す（フィールド名に使う）。
func goTypeName(t gotypes.Type) string {
	if named, ok := t.(*gotypes.Named); ok {
		return named.Obj().Name()
	}
	if pointer, ok := t.(*gotypes.Pointer); ok {
		return goTypeName(pointer.Elem())
	}
	return ""
}
