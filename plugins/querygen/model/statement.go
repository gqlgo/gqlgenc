package model

import (
	"fmt"
	"strings"
)

// Statement represents a code statement in the AST
type Statement interface {
	String(indent int) string
}

// VariableDecl represents a variable declaration
type VariableDecl struct {
	Name string
	Type string
}

func (v *VariableDecl) String(indent int) string {
	return fmt.Sprintf("var %s %s", v.Name, v.Type)
}

// IfStatement represents an if statement
type IfStatement struct {
	Condition string
	Body      []Statement
}

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

// SwitchStatement represents a switch statement
type SwitchStatement struct {
	Expr  string
	Cases []SwitchCase
}

type SwitchCase struct {
	Value string
	Body  []Statement
}

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

// Assignment represents an assignment statement
type Assignment struct {
	Target string
	Value  string
}

func (a *Assignment) String(indent int) string {
	return fmt.Sprintf("%s = %s", a.Target, a.Value)
}

// ReturnStatement represents a return statement
type ReturnStatement struct {
	Value string
}

func (r *ReturnStatement) String(indent int) string {
	if r.Value == "" {
		return "return"
	}
	return fmt.Sprintf("return %s", r.Value)
}

// RawStatement represents raw Go code
type RawStatement struct {
	Code string
}

func (r *RawStatement) String(indent int) string {
	return r.Code
}

// ErrorCheckStatement represents error checking pattern
type ErrorCheckStatement struct {
	ErrorExpr string
	Body      []Statement
}

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
