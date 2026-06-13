package querygen

import (
	"fmt"
	"strings"
)

// Statement は AST におけるコードステートメントを表す。
//
// String メソッドは指定されたインデントレベルで文字列表現を返す。
type Statement interface {
	String(indent int) string
}

// IfStatement は if 文を表す。
//
// 例:
//
//	if value, ok := raw["fieldName"]; ok {
//	    // Body
//	}
type IfStatement struct {
	Condition string      // 条件式
	Body      []Statement // if ブロック内のステートメント
}

// String は if 文の文字列表現を返す。
func (i *IfStatement) String(indent int) string {
	return renderBlock(indent, "if "+i.Condition, i.Body)
}

// SwitchStatement は switch 文を表す。
//
// 例:
//
//	switch typeName {
//	case "User":
//	    // Body
//	case "Post":
//	    // Body
//	}
type SwitchStatement struct {
	Expr  string       // switch の式
	Cases []SwitchCase // case のリスト
}

// SwitchCase は switch 文の単一の case を表す。
type SwitchCase struct {
	Value string      // case の値（例: case "User": における "User"）
	Body  []Statement // この case で実行するステートメント
}

// String は switch 文の文字列表現を返す。
func (s *SwitchStatement) String(indent int) string {
	var buf strings.Builder
	tabs := strings.Repeat("\t", indent)

	fmt.Fprintf(&buf, "switch %s {\n", s.Expr)
	for _, c := range s.Cases {
		fmt.Fprintf(&buf, "%scase %q:\n", tabs, c.Value)
		writeBody(&buf, indent, c.Body)
	}
	buf.WriteString(tabs + "}")

	return buf.String()
}

// Assignment は代入文を表す。
//
// 例: t.User = &UserFragment{}
type Assignment struct {
	Target string // 代入先
	Value  string // 代入する値
}

// String は代入文の文字列表現を返す。
func (a *Assignment) String(_ int) string {
	return fmt.Sprintf("%s = %s", a.Target, a.Value)
}

// ReturnStatement は return 文を表す。
//
// 例: return err
type ReturnStatement struct {
	Value string // 返す値（空の場合は単なる return）
}

// String は return 文の文字列表現を返す。
func (r *ReturnStatement) String(_ int) string {
	if r.Value == "" {
		return "return"
	}
	return fmt.Sprintf("return %s", r.Value)
}

// RawStatement は生の Go コードを表す。
//
// String() メソッドで文字列をそのまま返す。
type RawStatement struct {
	Code string // Go コード
}

// String は生のコードをそのまま返す。
func (r *RawStatement) String(_ int) string {
	return r.Code
}

// ErrorCheckStatement はエラーチェックパターンを表す。
//
// 例:
//
//	if err := json.Unmarshal(data, &t); err != nil {
//	    return err
//	}
type ErrorCheckStatement struct {
	ErrorExpr string      // エラーを返す式
	Body      []Statement // err != nil の場合に実行するステートメント
}

// String はエラーチェック文の文字列表現を返す。
func (e *ErrorCheckStatement) String(indent int) string {
	return renderBlock(indent, "if err := "+e.ErrorExpr+"; err != nil", e.Body)
}

// renderBlock は "<header> {" 行・1段深い body・閉じ括弧 "}" からなるブロックを描画する。
func renderBlock(indent int, header string, body []Statement) string {
	var buf strings.Builder
	tabs := strings.Repeat("\t", indent)

	buf.WriteString(header)
	buf.WriteString(" {\n")
	writeBody(&buf, indent, body)
	buf.WriteString(tabs + "}")

	return buf.String()
}

// writeBody は body の各ステートメントを、ブロックより1段深いインデントで書き込む。
func writeBody(buf *strings.Builder, indent int, body []Statement) {
	tabs := strings.Repeat("\t", indent)
	for _, stmt := range body {
		buf.WriteString(tabs + "\t")
		buf.WriteString(stmt.String(indent + 1))
		buf.WriteString("\n")
	}
}
