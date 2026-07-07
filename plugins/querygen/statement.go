package querygen

import (
	"fmt"
	"strings"
)

// Statement は生成コードの1ステートメントを表す。
//
// 出力は templates.Render が gofmt で再整形するため、String は手動でインデントを付けない。
type Statement interface {
	String() string
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
func (i *IfStatement) String() string {
	return renderBlock("if "+i.Condition, i.Body)
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
func (s *SwitchStatement) String() string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "switch %s {\n", s.Expr)
	for _, c := range s.Cases {
		fmt.Fprintf(&buf, "case %q:\n", c.Value)
		writeBody(&buf, c.Body)
	}
	buf.WriteString("}")

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
func (a *Assignment) String() string {
	return fmt.Sprintf("%s = %s", a.Target, a.Value)
}

// ReturnStatement は return 文を表す。
//
// 例: return err
type ReturnStatement struct {
	Value string // 返す値（空の場合は単なる return）
}

// String は return 文の文字列表現を返す。
func (r *ReturnStatement) String() string {
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
func (r *RawStatement) String() string {
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
func (e *ErrorCheckStatement) String() string {
	return renderBlock("if err := "+e.ErrorExpr+"; err != nil", e.Body)
}

// renderBlock は "<header> {" 行・body・閉じ括弧 "}" からなるブロックを描画する。
// インデントは付けない（gofmt が整形する）。
func renderBlock(header string, body []Statement) string {
	var buf strings.Builder

	buf.WriteString(header)
	buf.WriteString(" {\n")
	writeBody(&buf, body)
	buf.WriteString("}")

	return buf.String()
}

// writeBody は body の各ステートメントを1行ずつ書き込む（インデントなし）。
func writeBody(buf *strings.Builder, body []Statement) {
	for _, stmt := range body {
		buf.WriteString(stmt.String())
		buf.WriteString("\n")
	}
}
