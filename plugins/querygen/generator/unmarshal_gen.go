package generator

import (
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/builder"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/formatter"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// UnmarshalJSONGenerator generates UnmarshalJSON methods
type UnmarshalJSONGenerator struct {
	builder   *builder.UnmarshalBuilder
	formatter *formatter.CodeFormatter
}

// NewUnmarshalJSONGenerator creates a new UnmarshalJSONGenerator
func NewUnmarshalJSONGenerator() *UnmarshalJSONGenerator {
	return &UnmarshalJSONGenerator{
		builder:   builder.NewUnmarshalBuilder(),
		formatter: formatter.NewCodeFormatter(),
	}
}

// Generate generates an UnmarshalJSON method for the given type
func (g *UnmarshalJSONGenerator) Generate(typeInfo model.TypeInfo) string {
	if !typeInfo.ShouldGenerateUnmarshal {
		return ""
	}

	// Build method body statements
	statements := g.builder.BuildUnmarshalMethod(typeInfo)

	// Format into code
	return g.formatter.FormatUnmarshalMethod(typeInfo.TypeName, statements)
}
