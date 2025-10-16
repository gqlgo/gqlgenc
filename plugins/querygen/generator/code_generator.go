package generator

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/builder"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/formatter"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// CodeGenerator orchestrates all generators to produce complete type code
type CodeGenerator struct {
	analyzer         *TypeAnalyzer
	formatter        *formatter.CodeFormatter
	unmarshalBuilder *builder.UnmarshalBuilder
	typeCache        map[*types.Named]*model.TypeInfo
}

// NewCodeGenerator creates a new CodeGenerator
func NewCodeGenerator(goTypes []types.Type) *CodeGenerator {
	return &CodeGenerator{
		analyzer:         NewTypeAnalyzer(goTypes),
		formatter:        formatter.NewCodeFormatter(),
		unmarshalBuilder: builder.NewUnmarshalBuilder(),
		typeCache:        make(map[*types.Named]*model.TypeInfo),
	}
}

// Generate generates complete code for a type (type definition, UnmarshalJSON, getters)
func (g *CodeGenerator) Generate(t types.Type) (string, error) {
	typeInfo, err := g.analyzeType(t)
	if err != nil {
		return "", fmt.Errorf("failed to analyze type: %w", err)
	}

	parts := []string{g.generateTypeDecl(*typeInfo)}

	if unmarshal := g.generateUnmarshal(*typeInfo); unmarshal != "" {
		parts = append(parts, unmarshal)
	}

	if getters := g.generateGetters(*typeInfo); getters != "" {
		parts = append(parts, getters)
	}

	return strings.Join(parts, ""), nil
}

// NeedsJSONImport checks if any type needs JSON import
func (g *CodeGenerator) NeedsJSONImport(goTypes []types.Type) bool {
	for _, namedType := range g.analyzer.namedStructs(goTypes) {
		typeInfo, err := g.analyzeType(namedType)
		if err != nil {
			continue
		}
		if typeInfo.ShouldGenerateUnmarshal {
			return true
		}
	}
	return false
}

func (g *CodeGenerator) generateTypeDecl(typeInfo model.TypeInfo) string {
	return g.formatter.FormatTypeDecl(typeInfo.TypeName, typeInfo.Struct)
}

func (g *CodeGenerator) generateUnmarshal(typeInfo model.TypeInfo) string {
	if !typeInfo.ShouldGenerateUnmarshal {
		return ""
	}

	statements := g.unmarshalBuilder.BuildUnmarshalMethod(typeInfo)
	return g.formatter.FormatUnmarshalMethod(typeInfo.TypeName, statements)
}

func (g *CodeGenerator) generateGetters(typeInfo model.TypeInfo) string {
	var buf strings.Builder
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

func (g *CodeGenerator) analyzeType(t types.Type) (*model.TypeInfo, error) {
	if named := namedStructType(t); named != nil {
		if info, exists := g.typeCache[named]; exists {
			return info, nil
		}
	}

	info, err := g.analyzer.Analyze(t)
	if err != nil {
		return nil, err
	}

	if info.Named != nil {
		g.typeCache[info.Named] = info
	}

	return info, nil
}
