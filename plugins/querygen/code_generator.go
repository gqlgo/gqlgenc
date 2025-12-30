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

// emit は型定義、UnmarshalJSON メソッド、getter メソッドを含む
// 型の完全なコードを生成する。
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

// buildTypeInfo は Go 型を解析し、コード生成に必要な情報を抽出する。
// ポインタのアンラップを処理し、型が名前付き構造体型であることを検証する。
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

// buildFields は FieldAnalyzer を使用して構造体型の全フィールドを解析する。
// 埋め込みフィールド、インラインフラグメントなどを処理するアナライザに委譲する。
func (g *CodeGenerator) buildFields(structType *types.Struct) []FieldInfo {
	return g.analyzer.AnalyzeFields(structType, g.shouldGenerateUnmarshal)
}

// shouldGenerateUnmarshal は型に UnmarshalJSON メソッドを生成すべきかを判定する。
// 他の型に埋め込まれている型（fragment spreads）は生成をスキップする。
func (g *CodeGenerator) shouldGenerateUnmarshal(named *types.Named) bool {
	if named == nil {
		return false
	}

	_, skip := g.skipUnmarshal[named.Obj()]
	return !skip
}

// getStructType は様々な型ラッパーから構造体型を抽出する。
// Named 型と Pointer 型をアンラップして、基礎となる構造体に到達する。
//
// 型が構造体でない場合、または構造体にアンラップできない場合は nil を返す。
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

// namedStructType は構造体を基礎型として持つ Named 型を抽出する。
// Pointer 型をアンラップするが、構造体を基礎に持つ Named 型のみを返す。
//
// 型が名前付き構造体でない場合、または名前付き構造体にアンラップできない場合は nil を返す。
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

// collectEmbeddedTypes は埋め込み（匿名）フィールドとして使用されているすべての型を識別する。
//
// これらの型は親型の UnmarshalJSON メソッドを通じてアンマーシャルされるため、
// UnmarshalJSON の生成をスキップする必要がある。これにより、重複したアンマーシャル
// ロジックを防ぎ、fragment spreads が正しく動作することを保証する。
//
// 例えば、型 A が型 B を埋め込む場合:
//
//	type A struct {
//	    B  // 埋め込みフィールド（fragment spread）
//	    ID string
//	}
//
// この場合、B は返されるマップに含まれ、独自の UnmarshalJSON を生成しない。
// 代わりに、A の UnmarshalJSON が B のフィールドのアンマーシャルを処理する。
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
