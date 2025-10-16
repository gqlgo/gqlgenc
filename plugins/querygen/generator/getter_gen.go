package generator

import (
	"strings"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/formatter"
	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// GetterGenerator generates getter methods
type GetterGenerator struct {
	formatter *formatter.CodeFormatter
}

// NewGetterGenerator creates a new GetterGenerator
func NewGetterGenerator() *GetterGenerator {
	return &GetterGenerator{
		formatter: formatter.NewCodeFormatter(),
	}
}

// Generate generates getter methods for all fields in the type
func (g *GetterGenerator) Generate(typeInfo model.TypeInfo) string {
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
