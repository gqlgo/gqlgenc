package formatter

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// CodeFormatter formats generated code
type CodeFormatter struct{}

// NewCodeFormatter creates a new CodeFormatter
func NewCodeFormatter() *CodeFormatter {
	return &CodeFormatter{}
}

// FormatTypeDecl formats a type declaration
func (f *CodeFormatter) FormatTypeDecl(typeName string, structType *types.Struct) string {
	typeStr := templates.CurrentImports.LookupType(structType)
	return fmt.Sprintf("type %s %s\n", typeName, typeStr)
}

// FormatUnmarshalMethod formats an UnmarshalJSON method
func (f *CodeFormatter) FormatUnmarshalMethod(typeName string, body []model.Statement) string {
	var buf strings.Builder

	// Method signature
	buf.WriteString(fmt.Sprintf("func (t *%s) UnmarshalJSON(data []byte) error {\n", typeName))

	// Method body
	for _, stmt := range body {
		buf.WriteString("\t")
		buf.WriteString(stmt.String(1))
		buf.WriteString("\n")
	}

	// Closing
	buf.WriteString("}\n")

	return buf.String()
}

// FormatGetter formats a getter method
func (f *CodeFormatter) FormatGetter(typeName, fieldName, fieldType string) string {
	return fmt.Sprintf(`func (t *%s) Get%s() %s {
	if t == nil {
		t = &%s{}
	}
	return t.%s
}
`, typeName, fieldName, fieldType, typeName, fieldName)
}
