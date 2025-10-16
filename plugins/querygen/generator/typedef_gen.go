package generator

import (
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/formatter"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// TypeDefGenerator generates type definitions
type TypeDefGenerator struct {
	formatter *formatter.CodeFormatter
}

// NewTypeDefGenerator creates a new TypeDefGenerator
func NewTypeDefGenerator() *TypeDefGenerator {
	return &TypeDefGenerator{
		formatter: formatter.NewCodeFormatter(),
	}
}

// Generate generates a type definition
func (g *TypeDefGenerator) Generate(typeInfo model.TypeInfo) string {
	return g.formatter.FormatTypeDecl(typeInfo.TypeName, typeInfo.Struct)
}
