package querygen

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/99designs/gqlgen/codegen/templates"
)

// CodeFormatter は生成されるコードをフォーマットする。
type CodeFormatter struct{}

// NewCodeFormatter は新しい CodeFormatter を作成する。
func NewCodeFormatter() *CodeFormatter {
	return &CodeFormatter{}
}

// FormatTypeDecl は型定義を文字列にフォーマットする。
//
// パラメータ:
//   - typeName: 型名（例: "User"）
//   - structType: 構造体型の情報
//
// 戻り値: フォーマットされた型定義（例: "type User struct { ... }\n"）
func (f *CodeFormatter) FormatTypeDecl(typeName string, structType *types.Struct) string {
	typeStr := templates.CurrentImports.LookupType(structType)
	return fmt.Sprintf("type %s %s\n", typeName, typeStr)
}

// FormatUnmarshalMethod は UnmarshalJSON メソッドを文字列にフォーマットする。
//
// 生成される UnmarshalJSON メソッドは、GraphQL レスポンスの JSON データを
// 構造体にデシリアライズするために使用される。
//
// パラメータ:
//   - typeName: レシーバ型の名前（例: "User"）
//   - body: メソッド本体のステートメントリスト
//
// 戻り値: フォーマットされた UnmarshalJSON メソッド定義
func (f *CodeFormatter) FormatUnmarshalMethod(typeName string, body []Statement) string {
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

// FormatGetter は getter メソッドを文字列にフォーマットする。
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
func (f *CodeFormatter) FormatGetter(typeName, fieldName, fieldType string) string {
	return fmt.Sprintf(`func (t *%s) Get%s() %s {
	if t == nil {
		t = &%s{}
	}
	return t.%s
}
`, typeName, fieldName, fieldType, typeName, fieldName)
}
