package codegen

import (
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
}

func NewGoTypeGenerator(cfg *config.Config) *GoTypeGenerator {
	return &GoTypeGenerator{
		cfg:    cfg,
		binder: cfg.GQLGenConfig.NewBinder(),
		types:  map[string]gotypes.Type{},
	}
}

func (g *GoTypeGenerator) CreateGoTypes(operations graphql.OperationList) []gotypes.Type {
	for _, operation := range operations {
		t := g.newFields(operation.Name, operation.SelectionSet).goStructType()
		g.newGoNamedType(operation.Name, false, t)
	}

	return g.goTypes()
}

func (g *GoTypeGenerator) goTypes() []gotypes.Type {
	return slices.SortedFunc(maps.Values(g.types), func(a, b gotypes.Type) int {
		return strings.Compare(strings.TrimPrefix(a.String(), "*"), strings.TrimPrefix(b.String(), "*"))
	})
}

// When parentTypeName is empty, the parent is an inline fragment
func (g *GoTypeGenerator) newFields(parentTypeName string, selectionSet graphql.SelectionSet) Fields {
	fields := make(Fields, 0, len(selectionSet))
	for _, selection := range selectionSet {
		fields = append(fields, g.newField(parentTypeName, selection))
	}

	return fields
}

// When parentTypeName is empty, the parent is an inline fragment
func (g *GoTypeGenerator) newField(parentTypeName string, selection graphql.Selection) *Field {
	switch sel := selection.(type) {
	case *graphql.Field:
		typeKind, t := g.newTypeKindAndGoType(parentTypeName, sel)
		tags := []string{fmt.Sprintf(`json:"%s%s"`, sel.Alias, g.jsonOmitTag(sel))}
		return newField(typeKind, t, sel.Alias, tags)
	case *graphql.FragmentSpread:
		structType := g.newFields(sel.Name, sel.Definition.SelectionSet).goStructType()
		namedType := g.newGoNamedType(sel.Name, true, structType)
		return newField(FragmentSpread, namedType, sel.Name, []string{`json:"-"`})
	case *graphql.InlineFragment:
		structType := g.newFields("", sel.SelectionSet).goStructType()
		pointerType := gotypes.NewPointer(structType)
		tags := []string{`json:"-"`}
		return newField(InlineFragment, pointerType, sel.TypeCondition, tags)
	}
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
	goType, err := g.binder.FindTypeFromName(g.cfg.GQLGenConfig.Models[typeName].Model[0])
	if err != nil {
		// If we pass the correct typeName as per implementation, it should always be found, so we panic if not
		panic(fmt.Sprintf("%+v", err))
	}
	if !nonNull {
		goType = gotypes.NewPointer(goType)
	}

	return goType
}

func (g *GoTypeGenerator) jsonOmitTag(field *graphql.Field) string {
	var jsonOmitTag string
	if field.Definition.Type.NonNull {
		if g.cfg.GQLGenConfig.EnableModelJsonOmitzeroTag != nil && *g.cfg.GQLGenConfig.EnableModelJsonOmitzeroTag {
			jsonOmitTag += `,omitzero`
		}
	}
	return jsonOmitTag
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
	fieldMapByName := make(map[string]*Field, len(fs))
	for _, field := range fs {
		fieldMapByName[field.Name] = field
	}
	return slices.SortedFunc(maps.Values(fieldMapByName), func(a *Field, b *Field) int {
		return strings.Compare(a.Name, b.Name)
	})
}
