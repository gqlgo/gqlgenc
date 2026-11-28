package introspection

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	astparser "github.com/vektah/gqlparser/v2/parser"
)

// errIntrospectionTypeTooDeep は、introspection パースで唯一「仕様違反の応答ではなく valid な
// スキーマ」が原因で起きる panic の sentinel。型の list/non-null 入れ子が introspection クエリの
// ofType 深さ (7) を超えると getType が OfType=nil に当たる。SchemaFromIntrospection はこれだけを
// エラーに変換し、仕様違反由来の panic (kind 不一致・型欠落) はそのまま再 panic する。
var errIntrospectionTypeTooDeep = errors.New("type nested deeper than the introspection query's ofType depth (7); use a local schema (schema.files) instead of endpoint introspection for deeply nested list/non-null types")

func SchemaFromIntrospection(url string, query Query) (*ast.SchemaDocument, error) {
	parser := parser{
		sharedPosition: &ast.Position{Src: &ast.Source{
			Name:    "remote",
			BuiltIn: false,
		}},
		typeMap: query.Schema.Types.NameMap(),
	}

	if url != "" {
		parser.sharedPosition.Src.Name = url
	}

	var doc *ast.SchemaDocument
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(error); ok && errors.Is(e, errIntrospectionTypeTooDeep) {
					err = fmt.Errorf("introspection schema parse failed: %w", e)
					return
				}
				panic(r)
			}
		}()
		doc = parser.parseIntrospectionQuery(query)
	}()

	return doc, err
}

type parser struct {
	sharedPosition                *ast.Position
	typeMap                       map[string]*FullType
	deprecatedDirectiveDefinition *ast.DirectiveDefinition
}

func (p parser) parseIntrospectionQuery(query Query) *ast.SchemaDocument {
	var doc ast.SchemaDocument

	doc.Schema = append(doc.Schema, p.parseSchemaDefinition(query, p.typeMap))
	doc.Position = p.sharedPosition

	// parseDirectiveDefinition before parseTypeSystemDefinition
	// Because SystemDefinition depends on DirectiveDefinition
	for _, directiveValue := range query.Schema.Directives {
		doc.Directives = append(doc.Directives, p.parseDirectiveDefinition(directiveValue))
	}

	p.deprecatedDirectiveDefinition = doc.Directives.ForName("deprecated")

	for _, typeVale := range p.typeMap {
		doc.Definitions = append(doc.Definitions, p.parseTypeSystemDefinition(typeVale))
	}

	return &doc
}

func (p parser) parseSchemaDefinition(query Query, typeMap map[string]*FullType) *ast.SchemaDefinition {
	def := ast.SchemaDefinition{
		Position: p.sharedPosition}

	if query.Schema.QueryType.Name != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinition(ast.Query, typeMap[*query.Schema.QueryType.Name]),
		)
	}

	if query.Schema.MutationType != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinition(ast.Mutation, typeMap[*query.Schema.MutationType.Name]),
		)
	}

	if query.Schema.SubscriptionType != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinition(ast.Subscription, typeMap[*query.Schema.SubscriptionType.Name]),
		)
	}

	return &def
}

func (p parser) parseOperationTypeDefinition(operation ast.Operation, fullType *FullType) *ast.OperationTypeDefinition {
	return &ast.OperationTypeDefinition{
		Operation: operation,
		Type:      *fullType.Name,
		Position:  p.sharedPosition,
	}
}

func (p parser) parseDirectiveDefinition(directiveValue *DirectiveType) *ast.DirectiveDefinition {
	args := make(ast.ArgumentDefinitionList, 0, len(directiveValue.Args))

	for _, arg := range directiveValue.Args {
		argumentDefinition := p.buildInputValue(arg)
		args = append(args, argumentDefinition)
	}

	locations := make([]ast.DirectiveLocation, 0, len(directiveValue.Locations))
	for _, locationValue := range directiveValue.Locations {
		locations = append(locations, ast.DirectiveLocation(locationValue))
	}

	return &ast.DirectiveDefinition{
		Description: pointerString(directiveValue.Description),
		Name:        directiveValue.Name,
		Arguments:   args,
		Locations:   locations,
		// repeatable を落とすと、同じ位置に2回書いたディレクティブを持つクエリが検証で弾かれる
		IsRepeatable: directiveValue.IsRepeatable,
		Position:     p.sharedPosition,
	}
}

func (p parser) parseObjectFields(typeVale *FullType) ast.FieldList {
	fieldList := make(ast.FieldList, 0, len(typeVale.Fields))

	for _, field := range typeVale.Fields {
		typ := p.getType(&field.Type)
		args := make(ast.ArgumentDefinitionList, 0, len(field.Args))

		for _, arg := range field.Args {
			argumentDefinition := p.buildInputValue(arg)
			args = append(args, argumentDefinition)
		}

		fieldDefinition := &ast.FieldDefinition{
			Description: pointerString(field.Description),
			Name:        field.Name,
			Arguments:   args,
			Type:        typ,
			Position:    p.sharedPosition,
			Directives:  p.buildDeprecatedDirective(field.IsDeprecated, field.DeprecationReason, ast.LocationFieldDefinition),
		}
		fieldList = append(fieldList, fieldDefinition)
	}

	return fieldList
}

func (p parser) parseInputObjectFields(typeVale *FullType) ast.FieldList {
	fieldList := make(ast.FieldList, 0, len(typeVale.InputFields))

	for _, field := range typeVale.InputFields {
		typ := p.getType(&field.Type)
		fieldDefinition := &ast.FieldDefinition{
			Description: pointerString(field.Description),
			Name:        field.Name,
			// 既定値を落とすと、既定値を持つ必須の input フィールドを省いたクエリが検証で弾かれる
			DefaultValue: p.buildDefaultValue(field.DefaultValue, typ),
			Type:         typ,
			Position:     p.sharedPosition,
			Directives:   p.buildDeprecatedDirective(field.IsDeprecated, field.DeprecationReason, ast.LocationInputFieldDefinition),
		}
		fieldList = append(fieldList, fieldDefinition)
	}

	return fieldList
}

func (p parser) parseObjectTypeDefinition(typeVale *FullType) *ast.Definition {
	return &ast.Definition{
		Kind:        ast.Object,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		Interfaces:  interfaceNames(typeVale),
		Fields:      p.parseObjectFields(typeVale),
		Position:    p.sharedPosition,
		BuiltIn:     builtIn(typeVale),
	}
}

func (p parser) parseInterfaceTypeDefinition(typeVale *FullType) *ast.Definition {
	return &ast.Definition{
		Kind:        ast.Interface,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		Interfaces:  interfaceNames(typeVale),
		Fields:      p.parseObjectFields(typeVale),
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

func (p parser) parseInputObjectTypeDefinition(typeVale *FullType) *ast.Definition {
	return &ast.Definition{
		Kind:        ast.InputObject,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		Fields:      p.parseInputObjectFields(typeVale),
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

// interfaceNames は type が実装するインターフェース名の一覧を返す。
func interfaceNames(typeVale *FullType) []string {
	interfaces := make([]string, 0, len(typeVale.Interfaces))
	for _, intf := range typeVale.Interfaces {
		interfaces = append(interfaces, pointerString(intf.Name))
	}

	return interfaces
}

func (p parser) parseUnionTypeDefinition(typeVale *FullType) *ast.Definition {
	unions := make([]string, 0, len(typeVale.PossibleTypes))
	for _, unionValue := range typeVale.PossibleTypes {
		unions = append(unions, *unionValue.Name)
	}

	return &ast.Definition{
		Kind:        ast.Union,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		Types:       unions,
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

func (p parser) parseEnumTypeDefinition(typeVale *FullType) *ast.Definition {
	enums := make(ast.EnumValueList, 0, len(typeVale.EnumValues))

	for _, enum := range typeVale.EnumValues {
		enumValue := &ast.EnumValueDefinition{
			Description: pointerString(enum.Description),
			Name:        enum.Name,
			Position:    p.sharedPosition,
			Directives:  p.buildDeprecatedDirective(enum.IsDeprecated, enum.DeprecationReason, ast.LocationEnumValue),
		}
		enums = append(enums, enumValue)
	}

	return &ast.Definition{
		Kind:        ast.Enum,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		EnumValues:  enums,
		Position:    p.sharedPosition,
		BuiltIn:     builtIn(typeVale),
	}
}

func (p parser) parseScalarTypeExtension(typeVale *FullType) *ast.Definition {
	return &ast.Definition{
		Kind:        ast.Scalar,
		Description: pointerString(typeVale.Description),
		Name:        pointerString(typeVale.Name),
		Position:    p.sharedPosition,
		BuiltIn:     builtInScalar(typeVale),
	}
}

func (p parser) parseTypeSystemDefinition(typeVale *FullType) *ast.Definition {
	switch typeVale.Kind {
	case TypeKindScalar:
		return p.parseScalarTypeExtension(typeVale)
	case TypeKindInterface:
		return p.parseInterfaceTypeDefinition(typeVale)
	case TypeKindEnum:
		return p.parseEnumTypeDefinition(typeVale)
	case TypeKindUnion:
		return p.parseUnionTypeDefinition(typeVale)
	case TypeKindObject:
		return p.parseObjectTypeDefinition(typeVale)
	case TypeKindInputObject:
		return p.parseInputObjectTypeDefinition(typeVale)
	case TypeKindList, TypeKindNonNull:
		panic(fmt.Sprintf("not match Kind: %s", typeVale.Kind))
	}

	panic(fmt.Sprintf("not match Kind: %s", typeVale.Kind))
}

func (p parser) buildInputValue(input *InputValue) *ast.ArgumentDefinition {
	typ := p.getType(&input.Type)

	return &ast.ArgumentDefinition{
		Description:  pointerString(input.Description),
		Name:         input.Name,
		DefaultValue: p.buildDefaultValue(input.DefaultValue, typ),
		Type:         typ,
		Position:     p.sharedPosition,
		Directives:   p.buildDeprecatedDirective(input.IsDeprecated, input.DeprecationReason, ast.LocationArgumentDefinition),
	}
}

// buildDefaultValue は、引数または input フィールドの既定値を組み立てる。
//
// サーバーは既定値を GraphQL のリテラル (`"text"`・`[A, B]`・`{min: 1}`) の文字列で返す。
// リテラルとして解釈することで、文字列は引用符を外した値になり、リストは要素を、オブジェクトは
// フィールドを保つ。リテラルとして解釈できない値は、書かれたまま typ の種類の値として残す。
//
// 事前条件: typ はスキーマにある型を指す。
// 事後条件: defaultValue が nil なら nil を返す。返す値の Position は、入れ子の値も含めて
// スキーマの Position になる。
func (p parser) buildDefaultValue(defaultValue *string, typ *ast.Type) *ast.Value {
	if defaultValue == nil {
		return nil
	}

	value, err := parseValueLiteral(*defaultValue)
	if err != nil {
		return &ast.Value{
			Raw:      *defaultValue,
			Kind:     p.parseValueKind(typ),
			Position: p.sharedPosition,
		}
	}

	p.setPosition(value)

	return value
}

// setPosition は、value とその入れ子の値の Position をスキーマの Position にする。
// parseValueLiteral が返す値は、解釈のために作った一時的なソースを指しているためである。
func (p parser) setPosition(value *ast.Value) {
	value.Position = p.sharedPosition

	for _, child := range value.Children {
		child.Position = p.sharedPosition
		p.setPosition(child.Value)
	}
}

// parseValueLiteral は GraphQL の値のリテラルを解釈する。literal が 1 つの値でなければエラーを返す。
func parseValueLiteral(literal string) (*ast.Value, error) {
	// 値はドキュメントの一部としてしか解釈できないため、フィールドの引数として包む。
	// 改行は、literal が行コメントで終わっていても閉じ括弧を読めるようにするため。
	document, err := astparser.ParseQuery(&ast.Source{Input: "{field(argument: " + literal + "\n)}"})
	if err != nil {
		return nil, fmt.Errorf("parse value literal %q: %w", literal, err)
	}

	if len(document.Operations) != 1 || len(document.Operations[0].SelectionSet) != 1 {
		return nil, fmt.Errorf("value literal %q is not one value", literal)
	}

	field, ok := document.Operations[0].SelectionSet[0].(*ast.Field)
	if !ok || len(field.Arguments) != 1 {
		return nil, fmt.Errorf("value literal %q is not one value", literal)
	}

	return field.Arguments[0].Value, nil
}

func (p parser) getType(typeRef *TypeRef) *ast.Type {
	if typeRef.Kind == TypeKindList {
		itemRef := typeRef.OfType
		if itemRef == nil {
			panic(errIntrospectionTypeTooDeep)
		}

		return ast.ListType(p.getType(itemRef), p.sharedPosition)
	}

	if typeRef.Kind == TypeKindNonNull {
		nullableRef := typeRef.OfType
		if nullableRef == nil {
			panic(errIntrospectionTypeTooDeep)
		}

		nullableType := p.getType(nullableRef)
		nullableType.NonNull = true

		return nullableType
	}

	return ast.NamedType(pointerString(typeRef.Name), p.sharedPosition)
}

// buildDeprecatedDirective は、スキーマの要素 (フィールド・引数・input フィールド・enum の値) の
// @deprecated を組み立てる。isDeprecated が false なら nil を返す。location は、その要素に
// ディレクティブを書く位置を表す。
func (p parser) buildDeprecatedDirective(isDeprecated bool, deprecationReason *string, location ast.DirectiveLocation) ast.DirectiveList {
	var directives ast.DirectiveList

	if isDeprecated {
		var arguments ast.ArgumentList
		if deprecationReason != nil {
			arguments = append(arguments, &ast.Argument{
				Name: "reason",
				Value: &ast.Value{
					Raw:      *deprecationReason,
					Kind:     ast.StringValue,
					Position: p.sharedPosition,
				},
				Position: p.sharedPosition,
			})
		}

		deprecatedDirective := &ast.Directive{
			Name:             "deprecated",
			Arguments:        arguments,
			Position:         p.sharedPosition,
			ParentDefinition: nil,
			Definition:       p.deprecatedDirectiveDefinition,
			Location:         location,
		}
		directives = append(directives, deprecatedDirective)
	}

	return directives
}

func (p parser) parseValueKind(typ *ast.Type) ast.ValueKind {
	typName := typ.Name()

	if fullType, ok := p.typeMap[typName]; ok {
		switch fullType.Kind {
		case TypeKindEnum:
			return ast.EnumValue
		case TypeKindInputObject, TypeKindObject, TypeKindUnion, TypeKindInterface:
			return ast.ObjectValue
		case TypeKindList:
			return ast.ListValue
		case TypeKindNonNull:
			panic("parseValueKind not match Type Name: " + typ.Name())
		case TypeKindScalar:
			switch typName {
			case "Int":
				return ast.IntValue
			case "Float":
				return ast.FloatValue
			case "Boolean":
				return ast.BooleanValue
			case "String", "ID":
				return ast.StringValue
			default:
				return ast.StringValue
			}
		}
	}

	panic("parseValueKind not match Type Name: " + typ.Name())
}

func pointerString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// builtIn reports whether the type is an introspection meta type (name
// prefixed with "__").
func builtIn(fullType *FullType) bool {
	return strings.HasPrefix(pointerString(fullType.Name), "__")
}

func builtInScalar(fullType *FullType) bool {
	if builtIn(fullType) {
		return true
	}

	switch pointerString(fullType.Name) {
	case "String", "Int", "Float", "Boolean", "ID":
		return true
	}

	return false
}
