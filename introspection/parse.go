package introspection

import (
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
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
		Position:    p.sharedPosition,
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
			Directives:  p.buildDeprecatedDirective(field),
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
			Description: pointerString(field.Description),
			Name:        field.Name,
			Type:        typ,
			Position:    p.sharedPosition,
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

	var defaultValue *ast.Value
	if input.DefaultValue != nil {
		defaultValue = &ast.Value{
			Raw:      pointerString(input.DefaultValue),
			Kind:     p.parseValueKind(typ),
			Position: p.sharedPosition,
		}
	}

	return &ast.ArgumentDefinition{
		Description:  pointerString(input.Description),
		Name:         input.Name,
		DefaultValue: defaultValue,
		Type:         typ,
		Position:     p.sharedPosition,
	}
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

func (p parser) buildDeprecatedDirective(field *FieldValue) ast.DirectiveList {
	var directives ast.DirectiveList

	if field.IsDeprecated {
		var arguments ast.ArgumentList
		if field.DeprecationReason != nil {
			arguments = append(arguments, &ast.Argument{
				Name: "reason",
				Value: &ast.Value{
					Raw:      *field.DeprecationReason,
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
			Location:         ast.LocationVariableDefinition,
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
