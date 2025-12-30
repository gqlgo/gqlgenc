package querygen

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"
)

// CodeGenerator は全てのジェネレータを統合し、完全な型コードを生成する。
type CodeGenerator struct {
	unmarshalBuilder *UnmarshalBuilder
	analyzer         *FieldAnalyzer
	skipUnmarshal    map[*types.TypeName]struct{}
}

// NewCodeGenerator は新しい CodeGenerator を作成する。
//
// パラメータ:
//   - goTypes: 生成対象の全ての Go 型のリスト
//
// このコンストラクタは埋め込み型を識別し、それらの型に対する UnmarshalJSON の
// 生成をスキップするように設定する。
func NewCodeGenerator(goTypes []types.Type) *CodeGenerator {
	return &CodeGenerator{
		unmarshalBuilder: NewUnmarshalBuilder(),
		analyzer:         NewFieldAnalyzer(),
		skipUnmarshal:    findEmbeddedTypes(goTypes),
	}
}

// Generate は型の完全なコードを生成する（型定義、UnmarshalJSON、getter メソッド）。
//
// パラメータ:
//   - t: コード生成対象の Go 型
//
// 戻り値:
//   - string: 生成されたコード
//   - error: 型の解析に失敗した場合のエラー
func (g *CodeGenerator) Generate(t types.Type) (string, error) {
	typeInfo, err := g.analyzeType(t)
	if err != nil {
		return "", fmt.Errorf("failed to analyze type: %w", err)
	}

	return g.generateTypeCode(*typeInfo), nil
}

// NeedsJSONImport は、いずれかの型が JSON インポートを必要とするかを確認する。
//
// パラメータ:
//   - goTypes: チェック対象の Go 型のリスト
//
// 戻り値:
//   - bool: いずれかの型で UnmarshalJSON メソッドを生成する場合は true
func (g *CodeGenerator) NeedsJSONImport(goTypes []types.Type) bool {
	for _, t := range goTypes {
		typeInfo, err := g.analyzeType(t)
		if err != nil {
			continue
		}
		if typeInfo.ShouldGenerateUnmarshal {
			return true
		}
	}
	return false
}

// generateTypeCode は型定義、UnmarshalJSON メソッド、getter メソッドを含む
// 型の完全なコードを生成する。
//
// パラメータ:
//   - typeInfo: コード生成対象の型情報
//
// 戻り値:
//   - string: 生成された Go コード（型定義 + UnmarshalJSON + getters）
func (g *CodeGenerator) generateTypeCode(typeInfo TypeInfo) string {
	var buf strings.Builder

	buf.WriteString(g.formatTypeDecl(typeInfo.TypeName, typeInfo.Struct))

	if typeInfo.ShouldGenerateUnmarshal {
		statements := g.unmarshalBuilder.BuildUnmarshalMethod(typeInfo)
		buf.WriteString(g.formatUnmarshalMethod(typeInfo.TypeName, statements))
	}

	for _, field := range typeInfo.Fields {
		getter := g.formatGetter(
			typeInfo.TypeName,
			field.Name,
			field.TypeName,
		)
		buf.WriteString(getter)
	}
	return buf.String()
}

// analyzeType は Go 型を解析し、コード生成に必要な情報を返す。
//
// このメソッドは以下を実行する:
//   - ポインタ型のアンラップ
//   - 型が名前付き構造体型であることの検証
//   - フィールドの解析
//   - UnmarshalJSON 生成の必要性の判定
//
// パラメータ:
//   - t: 解析対象の Go 型
//
// 戻り値:
//   - *TypeInfo: 型情報
//   - error: 型が名前付き構造体でない場合のエラー
func (g *CodeGenerator) analyzeType(t types.Type) (*TypeInfo, error) {
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
	fields := g.analyzeStructFields(structType)
	shouldGenerate := g.shouldGenerateUnmarshal(namedType)

	return &TypeInfo{
		Named:                   namedType,
		Struct:                  structType,
		TypeName:                typeName,
		Fields:                  fields,
		ShouldGenerateUnmarshal: shouldGenerate,
	}, nil
}

// analyzeStructFields は FieldAnalyzer を使用して構造体型の全フィールドを解析する。
//
// このメソッドはフィールド解析を FieldAnalyzer に委譲し、埋め込みフィールド、
// インラインフラグメント、fragment spreads などを処理する。
//
// パラメータ:
//   - structType: 解析対象の構造体型
//
// 戻り値:
//   - []FieldInfo: 解析されたフィールド情報のリスト
func (g *CodeGenerator) analyzeStructFields(structType *types.Struct) []FieldInfo {
	return g.analyzer.AnalyzeFields(structType, g.shouldGenerateUnmarshal)
}

// shouldGenerateUnmarshal は型に UnmarshalJSON メソッドを生成すべきかを判定する。
//
// 他の型に埋め込まれている型（fragment spreads）は、親型の UnmarshalJSON で
// 処理されるため、独自の UnmarshalJSON の生成をスキップする。
//
// パラメータ:
//   - named: 判定対象の名前付き型
//
// 戻り値:
//   - bool: UnmarshalJSON を生成すべき場合は true
func (g *CodeGenerator) shouldGenerateUnmarshal(named *types.Named) bool {
	if named == nil {
		return false
	}

	_, skip := g.skipUnmarshal[named.Obj()]
	return !skip
}

// unwrapToStruct は様々な型ラッパーから構造体型を取得する。
//
// このメソッドは Named 型と Pointer 型を再帰的にアンラップして、
// 基礎となる構造体型に到達する。
//
// パラメータ:
//   - t: アンラップ対象の Go 型
//
// 戻り値:
//   - *types.Struct: 構造体型、または型が構造体でない場合は nil
func unwrapToStruct(t types.Type) *types.Struct {
	switch tt := t.(type) {
	case *types.Struct:
		return tt
	case *types.Named:
		if st, ok := tt.Underlying().(*types.Struct); ok {
			return st
		}
	case *types.Pointer:
		return unwrapToStruct(tt.Elem())
	}
	return nil
}

// unwrapToNamedStruct は構造体を基礎型として持つ Named 型を取得する。
//
// このメソッドは Pointer 型を再帰的にアンラップするが、
// 構造体を基礎に持つ Named 型のみを返す。
//
// パラメータ:
//   - t: アンラップ対象の Go 型
//
// 戻り値:
//   - *types.Named: 名前付き構造体型、または型が名前付き構造体でない場合は nil
func unwrapToNamedStruct(t types.Type) *types.Named {
	switch tt := t.(type) {
	case *types.Named:
		if _, ok := tt.Underlying().(*types.Struct); ok {
			return tt
		}
	case *types.Pointer:
		return unwrapToNamedStruct(tt.Elem())
	}
	return nil
}

// findEmbeddedTypes は埋め込み（匿名）フィールドとして使用されているすべての型を識別する。
//
// これらの型は親型の UnmarshalJSON メソッドを通じてアンマーシャルされるため、
// UnmarshalJSON の生成をスキップする必要がある。これにより、重複したアンマーシャル
// ロジックを防ぎ、GraphQL の fragment spreads が正しく動作することを保証する。
//
// 例:
//
//	type A struct {
//	    B  // 埋め込みフィールド（fragment spread）
//	    ID string
//	}
//
// この場合、B は返されるマップに含まれ、独自の UnmarshalJSON を生成しない。
// 代わりに、A の UnmarshalJSON が B のフィールドのアンマーシャルを処理する。
//
// パラメータ:
//   - goTypes: チェック対象の全ての Go 型のリスト
//
// 戻り値:
//   - map[*types.TypeName]struct{}: 埋め込みフィールドとして使用されている型名のセット
func findEmbeddedTypes(goTypes []types.Type) map[*types.TypeName]struct{} {
	result := make(map[*types.TypeName]struct{})
	for _, t := range goTypes {
		named := unwrapToNamedStruct(t)
		if named == nil {
			continue
		}
		structType := named.Underlying().(*types.Struct) //nolint:forcetypeassert // named.Underlying() is guaranteed to be *types.Struct by unwrapToNamedStruct
		for i := range structType.NumFields() {
			field := structType.Field(i)
			if !field.Anonymous() {
				continue
			}
			if namedField := unwrapToNamedStruct(field.Type()); namedField != nil {
				result[namedField.Obj()] = struct{}{}
			}
		}
	}
	return result
}

// formatTypeDecl は型定義を文字列にフォーマットする。
//
// パラメータ:
//   - typeName: 型名（例: "User"）
//   - structType: 構造体型の情報
//
// 戻り値: フォーマットされた型定義（例: "type User struct { ... }\n"）
func (g *CodeGenerator) formatTypeDecl(typeName string, structType *types.Struct) string {
	typeStr := templates.CurrentImports.LookupType(structType)
	return fmt.Sprintf("type %s %s\n", typeName, typeStr)
}

// formatUnmarshalMethod は UnmarshalJSON メソッドを文字列にフォーマットする。
//
// 生成される UnmarshalJSON メソッドは、GraphQL レスポンスの JSON データを
// 構造体にデシリアライズするために使用される。
//
// パラメータ:
//   - typeName: レシーバ型の名前（例: "User"）
//   - body: メソッド本体のステートメントリスト
//
// 戻り値: フォーマットされた UnmarshalJSON メソッド定義
func (g *CodeGenerator) formatUnmarshalMethod(typeName string, body []Statement) string {
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

// formatGetter は getter メソッドを文字列にフォーマットする。
//
// 生成される getter メソッドは nil セーフで、レシーバが nil の場合は
// ゼロ値で初期化された構造体を返す。
//
// パラメータ:
//   - typeName: レシーバ型の名前（例: "User"）
//   - fieldName: フィールド名（例: "Name"）
//   - fieldType: フィールドの型（例: "string"）
//
// 戻り値: フォーマットされた getter メソッド定義（例: "func (t *User) GetName() string { ... }"）
func (g *CodeGenerator) formatGetter(typeName, fieldName, fieldType string) string {
	return fmt.Sprintf(`func (t *%s) Get%s() %s {
	if t == nil {
		t = &%s{}
	}
	return t.%s
}
`, typeName, fieldName, fieldType, typeName, fieldName)
}
