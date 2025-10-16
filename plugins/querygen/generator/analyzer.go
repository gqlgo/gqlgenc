package generator

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"

	"github.com/Yamashou/gqlgenc/v3/plugins/querygen/model"
)

// TypeAnalyzer analyzes Go types and creates TypeInfo for code generation
type TypeAnalyzer struct {
	skipUnmarshalTypes map[*types.TypeName]struct{}
}

// NewTypeAnalyzer creates a new TypeAnalyzer
func NewTypeAnalyzer(goTypes []types.Type) *TypeAnalyzer {
	return &TypeAnalyzer{
		skipUnmarshalTypes: collectEmbeddedTypes(goTypes),
	}
}

// Analyze analyzes a type and returns TypeInfo
func (a *TypeAnalyzer) Analyze(t types.Type) (*model.TypeInfo, error) {
	// Handle pointer types
	if pointerType, ok := t.(*types.Pointer); ok {
		t = pointerType.Elem()
	}

	// Must be a named type
	namedType, ok := t.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("type must be named type: %v", t)
	}

	// Must have struct underlying type
	structType, ok := namedType.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("type must have struct underlying: %v", t)
	}

	typeName := templates.CurrentImports.LookupType(namedType)

	// Analyze fields
	fields := a.analyzeFields(structType)

	return &model.TypeInfo{
		Named:                   namedType,
		Struct:                  structType,
		TypeName:                typeName,
		Fields:                  fields,
		ShouldGenerateUnmarshal: a.shouldGenerateUnmarshal(namedType),
	}, nil
}

// namedStructs extracts named struct types from the provided list, handling pointers.
func (a *TypeAnalyzer) namedStructs(goTypes []types.Type) []*types.Named {
	var result []*types.Named
	for _, t := range goTypes {
		if named := namedStructType(t); named != nil {
			result = append(result, named)
		}
	}
	return result
}

// analyzeFields analyzes all fields in a struct
func (a *TypeAnalyzer) analyzeFields(structType *types.Struct) []model.FieldInfo {
	var fields []model.FieldInfo

	for i := range structType.NumFields() {
		field := structType.Field(i)
		tag := structType.Tag(i)

		fieldInfo := model.FieldInfo{
			Name:       field.Name(),
			Type:       field.Type(),
			TypeName:   templates.CurrentImports.LookupType(field.Type()),
			JSONTag:    a.parseJSONTag(tag),
			IsExported: field.Exported(),
			IsEmbedded: field.Anonymous(),
		}

		// Check if this is an inline fragment field
		if a.isInlineFragmentField(field, tag) {
			fieldInfo.IsInlineFragment = true

			// For pointer types, extract the element type
			if ptrType, ok := field.Type().(*types.Pointer); ok {
				fieldInfo.IsPointer = true
				elemType := ptrType.Elem()
				fieldInfo.PointerElemType = templates.CurrentImports.LookupType(elemType)
			}
		}

		// For embedded non-pointer fields with json:"-", analyze sub-fields
		// Only if the embedded type doesn't have its own UnmarshalJSON
		if fieldInfo.IsEmbedded && !fieldInfo.IsInlineFragment {
			// Check if embedded type will generate its own UnmarshalJSON
			if embeddedNamed := namedStructType(field.Type()); embeddedNamed != nil {
				// If embedded type generates UnmarshalJSON, it handles its own unmarshaling
				// So we don't need to recursively decode its fields
				if a.shouldGenerateUnmarshal(embeddedNamed) {
					// Skip sub-field analysis - the embedded type's UnmarshalJSON will handle it
					fields = append(fields, fieldInfo)
					continue
				}
			}

			// The embedded type doesn't have UnmarshalJSON, so we need to recursively decode its fields
			if embeddedStruct := a.getStructType(field.Type()); embeddedStruct != nil {
				fieldInfo.SubFields = a.analyzeFields(embeddedStruct)
			}
		}

		fields = append(fields, fieldInfo)
	}

	return fields
}

// getStructType extracts *types.Struct from a type
func (a *TypeAnalyzer) getStructType(t types.Type) *types.Struct {
	switch tt := t.(type) {
	case *types.Struct:
		return tt
	case *types.Named:
		if st, ok := tt.Underlying().(*types.Struct); ok {
			return st
		}
	case *types.Pointer:
		return a.getStructType(tt.Elem())
	}
	return nil
}

// isInlineFragmentField checks if a field is an inline fragment
func (a *TypeAnalyzer) isInlineFragmentField(field *types.Var, tag string) bool {
	if !field.Exported() {
		return false
	}

	jsonTag := a.parseJSONTag(tag)

	// Inline fragments have json:"-" or no json tag
	if jsonTag != "" && jsonTag != "-" {
		return false
	}

	// Check if field type is a pointer (inline fragment pattern)
	_, isPointer := field.Type().(*types.Pointer)
	return isPointer
}

// parseJSONTag extracts the JSON field name from struct tag
func (a *TypeAnalyzer) parseJSONTag(tag string) string {
	if tag == "" {
		return ""
	}
	value := reflect.StructTag(tag).Get("json")
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return value
}

// shouldGenerateUnmarshal determines if UnmarshalJSON should be generated
func (a *TypeAnalyzer) shouldGenerateUnmarshal(named *types.Named) bool {
	if named == nil {
		return false
	}
	_, skip := a.skipUnmarshalTypes[named.Obj()]
	return !skip
}

// collectEmbeddedTypes collects all embedded (anonymous) types that should skip UnmarshalJSON generation
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

// namedStructType extracts *types.Named from a type if it has struct underlying
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
