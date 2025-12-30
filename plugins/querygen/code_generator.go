package querygen

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"
)

// CodeGenerator orchestrates all generators to produce complete type code
type CodeGenerator struct {
	formatter        *CodeFormatter
	unmarshalBuilder *UnmarshalBuilder
	classifier       *FieldClassifier
	analyzer         *FieldAnalyzer
	skipUnmarshal    map[*types.TypeName]struct{}
}

// NewCodeGenerator creates a new CodeGenerator
func NewCodeGenerator(goTypes []types.Type) *CodeGenerator {
	return &CodeGenerator{
		formatter:        NewCodeFormatter(),
		unmarshalBuilder: NewUnmarshalBuilder(),
		classifier:       NewFieldClassifier(),
		analyzer:         NewFieldAnalyzer(),
		skipUnmarshal:    collectEmbeddedTypes(goTypes),
	}
}

// Generate generates complete code for a type (type definition, UnmarshalJSON, getters)
func (g *CodeGenerator) Generate(t types.Type) (string, error) {
	typeInfo, err := g.buildTypeInfo(t)
	if err != nil {
		return "", fmt.Errorf("failed to analyze type: %w", err)
	}

	return g.emit(*typeInfo), nil
}

// NeedsJSONImport checks if any type needs JSON import
func (g *CodeGenerator) NeedsJSONImport(goTypes []types.Type) bool {
	for _, t := range goTypes {
		typeInfo, err := g.buildTypeInfo(t)
		if err != nil {
			continue
		}
		if typeInfo.ShouldGenerateUnmarshal {
			return true
		}
	}
	return false
}

func (g *CodeGenerator) emit(typeInfo TypeInfo) string {
	var buf strings.Builder

	buf.WriteString(g.formatter.FormatTypeDecl(typeInfo.TypeName, typeInfo.Struct))

	if typeInfo.ShouldGenerateUnmarshal {
		statements := g.unmarshalBuilder.BuildUnmarshalMethod(typeInfo)
		buf.WriteString(g.formatter.FormatUnmarshalMethod(typeInfo.TypeName, statements))
	}

	for _, field := range typeInfo.Fields {
		getter := g.formatter.FormatGetter(
			typeInfo.TypeName,
			field.Name,
			field.TypeName,
		)
		buf.WriteString(getter)
	}
	return buf.String()
}

func (g *CodeGenerator) buildTypeInfo(t types.Type) (*TypeInfo, error) {
	if pointerType, ok := t.(*types.Pointer); ok {
		t = pointerType.Elem()
	}

	namedType, ok := t.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("type must be named type: %v", t)
	}

	structType, ok := namedType.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("type must have struct underlying: %v", t)
	}

	typeName := templates.CurrentImports.LookupType(namedType)
	fields := g.buildFields(structType)
	shouldGenerate := g.shouldGenerateUnmarshal(namedType)

	return &TypeInfo{
		Named:                   namedType,
		Struct:                  structType,
		TypeName:                typeName,
		Fields:                  fields,
		ShouldGenerateUnmarshal: shouldGenerate,
	}, nil
}

func (g *CodeGenerator) buildFields(structType *types.Struct) []FieldInfo {
	return g.analyzer.AnalyzeFields(structType, g.shouldGenerateUnmarshal)
}

func (g *CodeGenerator) shouldGenerateUnmarshal(named *types.Named) bool {
	if named == nil {
		return false
	}

	_, skip := g.skipUnmarshal[named.Obj()]
	return !skip
}

func getStructType(t types.Type) *types.Struct {
	switch tt := t.(type) {
	case *types.Struct:
		return tt
	case *types.Named:
		if st, ok := tt.Underlying().(*types.Struct); ok {
			return st
		}
	case *types.Pointer:
		return getStructType(tt.Elem())
	}
	return nil
}

func namedStructType(t types.Type) *types.Named {
	switch tt := t.(type) {
	case *types.Named:
		if _, ok := tt.Underlying().(*types.Struct); ok {
			return tt
		}
	case *types.Pointer:
		return namedStructType(tt.Elem())
	}
	return nil
}

func collectEmbeddedTypes(goTypes []types.Type) map[*types.TypeName]struct{} {
	result := make(map[*types.TypeName]struct{})
	for _, t := range goTypes {
		named := namedStructType(t)
		if named == nil {
			continue
		}
		structType := named.Underlying().(*types.Struct) //nolint:forcetypeassert // named.Underlying() is guaranteed to be *types.Struct by namedStructType
		for i := range structType.NumFields() {
			field := structType.Field(i)
			if !field.Anonymous() {
				continue
			}
			if namedField := namedStructType(field.Type()); namedField != nil {
				result[namedField.Obj()] = struct{}{}
			}
		}
	}
	return result
}
