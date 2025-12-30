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

// VariableDecl は変数宣言を表す。
//
// 例: var raw map[string]jsontext.Value
type VariableDecl struct {
	Name string // 変数名
	Type string // 変数の型
}

// String は変数宣言の文字列表現を返す。
func (v *VariableDecl) String(_ int) string {
	return fmt.Sprintf("var %s %s", v.Name, v.Type)
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
	var buf strings.Builder
	tabs := strings.Repeat("\t", indent)

	buf.WriteString(fmt.Sprintf("if %s {\n", i.Condition))
	for _, stmt := range i.Body {
		buf.WriteString(tabs + "\t")
		buf.WriteString(stmt.String(indent + 1))
		buf.WriteString("\n")
	}
	buf.WriteString(tabs + "}")

	return buf.String()
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

	buf.WriteString(fmt.Sprintf("switch %s {\n", s.Expr))
	for _, c := range s.Cases {
		buf.WriteString(tabs + fmt.Sprintf("case %q:\n", c.Value))
		for _, stmt := range c.Body {
			buf.WriteString(tabs + "\t")
			buf.WriteString(stmt.String(indent + 1))
			buf.WriteString("\n")
		}
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
	var buf strings.Builder
	tabs := strings.Repeat("\t", indent)

	buf.WriteString(fmt.Sprintf("if err := %s; err != nil {\n", e.ErrorExpr))
	for _, stmt := range e.Body {
		buf.WriteString(tabs + "\t")
		buf.WriteString(stmt.String(indent + 1))
		buf.WriteString("\n")
	}
	buf.WriteString(tabs + "}")

	return buf.String()
}
