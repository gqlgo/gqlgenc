package generator

import (
	"fmt"
	"go/types"
	"strings"
)

// CodeGenerator orchestrates all generators to produce complete type code
type CodeGenerator struct {
	analyzer      *TypeAnalyzer
	typeDefGen    *TypeDefGenerator
	unmarshalGen  *UnmarshalJSONGenerator
	getterGen     *GetterGenerator
}

// NewCodeGenerator creates a new CodeGenerator
func NewCodeGenerator(goTypes []types.Type) *CodeGenerator {
	return &CodeGenerator{
		analyzer:     NewTypeAnalyzer(goTypes),
		typeDefGen:   NewTypeDefGenerator(),
		unmarshalGen: NewUnmarshalJSONGenerator(),
		getterGen:    NewGetterGenerator(),
	}
}

// Generate generates complete code for a type (type definition, UnmarshalJSON, getters)
func (g *CodeGenerator) Generate(t types.Type) (string, error) {
	// Analyze type
	typeInfo, err := g.analyzer.Analyze(t)
	if err != nil {
		return "", fmt.Errorf("failed to analyze type: %w", err)
	}

	var buf strings.Builder

	// 1. Generate type definition
	typeDef := g.typeDefGen.Generate(*typeInfo)
	buf.WriteString(typeDef)

	// 2. Generate UnmarshalJSON method if needed
	unmarshalMethod := g.unmarshalGen.Generate(*typeInfo)
	if unmarshalMethod != "" {
		buf.WriteString(unmarshalMethod)
	}

	// 3. Generate getter methods
	getters := g.getterGen.Generate(*typeInfo)
	buf.WriteString(getters)

	return buf.String(), nil
}

// NeedsJSONImport checks if any type needs JSON import
func (g *CodeGenerator) NeedsJSONImport(goTypes []types.Type) bool {
	for _, t := range goTypes {
		// Handle pointer types
		if pointerType, ok := t.(*types.Pointer); ok {
			t = pointerType.Elem()
		}

		// Must be a named type with struct underlying
		namedType, ok := t.(*types.Named)
		if !ok {
			continue
		}
		if _, ok := namedType.Underlying().(*types.Struct); !ok {
			continue
		}

		// Check if we should generate unmarshal
		if g.analyzer.shouldGenerateUnmarshal(namedType) {
			return true
		}
	}
	return false
}
