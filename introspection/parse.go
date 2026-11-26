package introspection

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	astparser "github.com/vektah/gqlparser/v2/parser"
)

func ParseIntrospectionQuery(url string, query Query) *ast.SchemaDocument {
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

	return parser.parseIntrospectionQuery(query)
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

	for _, typeValue := range p.typeMap {
		doc.Definitions = append(doc.Definitions, p.parseTypeSystemDefinition(typeValue))
	}

	return &doc
}

func (p parser) parseSchemaDefinition(query Query, typeMap map[string]*FullType) *ast.SchemaDefinition {
	def := ast.SchemaDefinition{Position: p.sharedPosition}

	if query.Schema.QueryType.Name != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinitionForQuery(typeMap[*query.Schema.QueryType.Name]),
		)
	}

	if query.Schema.MutationType != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinitionForMutation(typeMap[*query.Schema.MutationType.Name]),
		)
	}

	if query.Schema.SubscriptionType != nil {
		def.OperationTypes = append(def.OperationTypes,
			p.parseOperationTypeDefinitionForSubscription(typeMap[*query.Schema.SubscriptionType.Name]),
		)
	}

	return &def
}

func (p parser) parseOperationTypeDefinitionForQuery(fullType *FullType) *ast.OperationTypeDefinition {
	var op ast.OperationTypeDefinition

	op.Operation = ast.Query
	op.Type = *fullType.Name
	op.Position = p.sharedPosition

	return &op
}

func (p parser) parseOperationTypeDefinitionForMutation(fullType *FullType) *ast.OperationTypeDefinition {
	var op ast.OperationTypeDefinition

	op.Operation = ast.Mutation
	op.Type = *fullType.Name
	op.Position = p.sharedPosition

	return &op
}

func (p parser) parseOperationTypeDefinitionForSubscription(fullType *FullType) *ast.OperationTypeDefinition {
	var op ast.OperationTypeDefinition

	op.Operation = ast.Subscription
	op.Type = *fullType.Name
	op.Position = p.sharedPosition

	return &op
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
		Description:  pointerString(directiveValue.Description),
		Name:         directiveValue.Name,
		Arguments:    args,
		Locations:    locations,
		IsRepeatable: directiveValue.IsRepeatable,
		Position:     p.sharedPosition,
	}
}

func (p parser) parseObjectFields(typeValue *FullType) ast.FieldList {
	fieldList := make(ast.FieldList, 0, len(typeValue.Fields))
	for _, field := range typeValue.Fields {
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

func (p parser) parseInputObjectFields(typeValue *FullType) ast.FieldList {
	fieldList := make(ast.FieldList, 0, len(typeValue.InputFields))
	for _, field := range typeValue.InputFields {
		typ := p.getType(&field.Type)
		fieldDefinition := &ast.FieldDefinition{
			Description:  pointerString(field.Description),
			Name:         field.Name,
			DefaultValue: p.buildDefaultValue(field.DefaultValue, typ),
			Type:         typ,
			Position:     p.sharedPosition,
			Directives:   p.buildDeprecatedDirective(field.IsDeprecated, field.DeprecationReason, ast.LocationInputFieldDefinition),
		}
		fieldList = append(fieldList, fieldDefinition)
	}

	return fieldList
}

func (p parser) parseObjectTypeDefinition(typeValue *FullType) *ast.Definition {
	fieldList := p.parseObjectFields(typeValue)

	interfaces := interfaceNames(typeValue)

	enums := make(ast.EnumValueList, 0, len(typeValue.EnumValues))
	for _, enum := range typeValue.EnumValues {
		enumValue := &ast.EnumValueDefinition{
			Description: pointerString(enum.Description),
			Name:        enum.Name,
			Position:    p.sharedPosition,
		}
		enums = append(enums, enumValue)
	}

	return &ast.Definition{
		Kind:        ast.Object,
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		Interfaces:  interfaces,
		Fields:      fieldList,
		EnumValues:  enums,
		Position:    p.sharedPosition,
		BuiltIn:     builtInObject(typeValue),
	}
}

func (p parser) parseInterfaceTypeDefinition(typeValue *FullType) *ast.Definition {
	fieldList := p.parseObjectFields(typeValue)

	interfaces := interfaceNames(typeValue)

	return &ast.Definition{
		Kind:        ast.Interface,
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		Interfaces:  interfaces,
		Fields:      fieldList,
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

func (p parser) parseInputObjectTypeDefinition(typeValue *FullType) *ast.Definition {
	fieldList := p.parseInputObjectFields(typeValue)

	interfaces := interfaceNames(typeValue)

	return &ast.Definition{
		Kind:        ast.InputObject,
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		Interfaces:  interfaces,
		Fields:      fieldList,
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

func (p parser) parseUnionTypeDefinition(typeValue *FullType) *ast.Definition {
	unions := make([]string, 0, len(typeValue.PossibleTypes))
	for _, unionValue := range typeValue.PossibleTypes {
		unions = append(unions, *unionValue.Name)
	}

	return &ast.Definition{
		Kind:        ast.Union,
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		Types:       unions,
		Position:    p.sharedPosition,
		BuiltIn:     false,
	}
}

func (p parser) parseEnumTypeDefinition(typeValue *FullType) *ast.Definition {
	enums := make(ast.EnumValueList, 0, len(typeValue.EnumValues))
	for _, enum := range typeValue.EnumValues {
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
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		EnumValues:  enums,
		Position:    p.sharedPosition,
		BuiltIn:     builtInEnum(typeValue),
	}
}

func (p parser) parseScalarTypeExtension(typeValue *FullType) *ast.Definition {
	return &ast.Definition{
		Kind:        ast.Scalar,
		Description: pointerString(typeValue.Description),
		Name:        pointerString(typeValue.Name),
		Position:    p.sharedPosition,
		BuiltIn:     builtInScalar(typeValue),
	}
}

func (p parser) parseTypeSystemDefinition(typeValue *FullType) *ast.Definition {
	switch typeValue.Kind {
	case TypeKindScalar:
		return p.parseScalarTypeExtension(typeValue)
	case TypeKindInterface:
		return p.parseInterfaceTypeDefinition(typeValue)
	case TypeKindEnum:
		return p.parseEnumTypeDefinition(typeValue)
	case TypeKindUnion:
		return p.parseUnionTypeDefinition(typeValue)
	case TypeKindObject:
		return p.parseObjectTypeDefinition(typeValue)
	case TypeKindInputObject:
		return p.parseInputObjectTypeDefinition(typeValue)
	case TypeKindList, TypeKindNonNull:
		panic(fmt.Sprintf("not match Kind: %s", typeValue.Kind))
	}

	panic(fmt.Sprintf("not match Kind: %s", typeValue.Kind))
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

// buildDefaultValue builds the default value of an argument or an input field.
//
// Arguments:
//   - defaultValue: the default value the server reports as a GraphQL literal, or nil when there is none
//   - typ: the type of the argument or the input field
//
// Returns:
//   - *ast.Value: the default value, or nil when defaultValue is nil
//
// Preconditions:
//   - typ names a type of the schema
//
// Postconditions:
//   - a literal that parses keeps its structure: a list has its items and an object has its fields
//   - a literal that does not parse is kept as written, with the kind of the named type of typ
//   - every position in the result is the position of the schema
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

// setPosition points a value and the values nested in it at the schema.
//
// Arguments:
//   - value: the value to update
//
// Returns:
//   - none
//
// Preconditions:
//   - value is not nil
//
// Postconditions:
//   - value, its children and their names have the position of the schema
func (p parser) setPosition(value *ast.Value) {
	value.Position = p.sharedPosition

	for _, child := range value.Children {
		child.Position = p.sharedPosition
		p.setPosition(child.Value)
	}
}

// parseValueLiteral parses a GraphQL value literal.
//
// Arguments:
//   - literal: the literal, such as `"text"`, `[A, B]` or `{min: 1}`
//
// Returns:
//   - *ast.Value: the parsed value
//   - error: non-nil when literal is not one GraphQL value
//
// Preconditions:
//   - none
//
// Postconditions:
//   - the positions in the value refer to a temporary source, not to the schema
func parseValueLiteral(literal string) (*ast.Value, error) {
	// A value can only be parsed as part of a document, so wrap it as an argument.
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
			panic("Decorated type deeper than introspection query.")
		}

		return ast.ListType(p.getType(itemRef), p.sharedPosition)
	}

	if typeRef.Kind == TypeKindNonNull {
		nullableRef := typeRef.OfType
		if nullableRef == nil {
			panic("Decorated type deeper than introspection query.")
		}

		nullableType := p.getType(nullableRef)
		nullableType.NonNull = true

		return nullableType
	}

	return ast.NamedType(pointerString(typeRef.Name), p.sharedPosition)
}

// buildDeprecatedDirective builds the @deprecated directive of a schema member.
//
// Arguments:
//   - isDeprecated: whether the server reports the member as deprecated
//   - deprecationReason: the reason the server reports, or nil when it reports none
//   - location: where the member is declared
//
// Returns:
//   - ast.DirectiveList: one @deprecated directive, or nil when isDeprecated is false
//
// Preconditions:
//   - the directive definitions have been parsed
//
// Postconditions:
//   - the directive has a reason argument only when deprecationReason is non-nil
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
			panic(fmt.Sprintf("parseValueKind not match Type Name: %s", typ.Name()))
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

	panic(fmt.Sprintf("parseValueKind not match Type Name: %s", typ.Name()))
}

func pointerString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func builtInScalar(fullType *FullType) bool {
	name := pointerString(fullType.Name)
	if strings.HasPrefix(name, "__") {
		return true
	}

	switch name {
	case "String", "Int", "Float", "Boolean", "ID":
		return true
	}

	return false
}

func builtInEnum(fullType *FullType) bool {
	name := pointerString(fullType.Name)

	return strings.HasPrefix(name, "__")
}

// interfaceNames collects the names of the interfaces the type implements
func interfaceNames(fullType *FullType) []string {
	interfaces := make([]string, 0, len(fullType.Interfaces))
	for _, intf := range fullType.Interfaces {
		interfaces = append(interfaces, pointerString(intf.Name))
	}

	return interfaces
}

func builtInObject(fullType *FullType) bool {
	name := pointerString(fullType.Name)

	return strings.HasPrefix(name, "__")
}
