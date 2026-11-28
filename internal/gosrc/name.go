// Package gosrc は、生成する Go のソースを書くための道具を持つ。
package gosrc

import (
	"strings"
	"unicode"
)

// initialisms は、Go の名前の中で大文字のまま書く頭字語。gqlgen の templates.CommonInitialisms と
// 同じ一覧にする。model の生成は gqlgen の modelgen が行うため、同じ GraphQL の名前から gqlgenc と
// modelgen が別の Go の名前を作らないようにするためである。
var initialisms = map[string]bool{
	"ACL":   true,
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"CSV":   true,
	"DNS":   true,
	"EOF":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ICMP":  true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"KVK":   true,
	"LHS":   true,
	"PDF":   true,
	"PGP":   true,
	"QPS":   true,
	"QR":    true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"SVG":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"UUID":  true,
	"VM":    true,
	"XML":   true,
	"XMPP":  true,
	"XSRF":  true,
	"XSS":   true,
	"AWS":   true,
	"GCP":   true,
}

// ToGo は、GraphQL の名前を、公開する Go の名前にする。区切り (_・-・空白) と小文字から大文字への
// 変わり目で語に分け、語の頭を大文字にしてつなぐ。頭字語 (ID・URL など) は大文字のままにする。
//
//	getUser → GetUser、user_id → UserID、TO_CAMEL → ToCamel、RelatedURLs → RelatedURLs
//
// "_" は "_" のまま返す。結果は gqlgen の templates.ToGo と同じになる (同じ規則を写したもの)。
func ToGo(name string) string {
	if name == "_" {
		return "_"
	}

	var goName strings.Builder
	walkWords(name, func(w word) {
		goName.WriteString(w.goSpelling())
	})

	return goName.String()
}

// word は、名前を分けた語の 1 つ。
type word struct {
	// text は語の元のつづり
	text string
	// isInitialism は、語が頭字語そのものか (id・ID・Id はどれも頭字語 ID)
	isInitialism bool
	// hasInitialism は、語が頭字語から始まるか (URLs は頭字語 URL から始まる)
	hasInitialism bool
}

// goSpelling は、語を Go の名前の中でのつづりにする。
func (w word) goSpelling() string {
	switch {
	case w.isInitialism:
		// id → ID
		return strings.ToUpper(w.text)
	case !w.hasInitialism && (strings.ToUpper(w.text) == w.text || strings.ToLower(w.text) == w.text):
		// FOO・foo → Foo。大文字と小文字が混ざる語 (FOo) と、頭字語から始まる語 (URLs) はそのまま
		return upperFirst(strings.ToLower(w.text))
	default:
		return w.text
	}
}

// walkWords は、name を語に分け、語ごとに f を呼ぶ。
//
// golint の lintName (https://github.com/golang/lint/blob/06c8688daad7faa9da5a0c2f163a3d14aac986ca/lint.go#L679)
// をもとにした gqlgen の wordWalker を写したもの。
func walkWords(name string, f func(word)) {
	runes := []rune(strings.TrimFunc(name, isDelimiter))
	start, i := 0, 0 // 語の先頭の位置・読んでいる位置
	hasInitialism := false
	for i+1 <= len(runes) {
		endOfWord := false
		switch {
		case i+1 == len(runes):
			endOfWord = true
		case isDelimiter(runes[i+1]):
			// 区切りの並びを読み飛ばし、残りを前へ詰める
			endOfWord = true
			n := 1
			for i+n+1 < len(runes) && isDelimiter(runes[i+n+1]) {
				n++
			}

			// 数字と数字の間の区切りは、1 つだけ残す (v1_2 を v12 にしない)
			if i+n+1 < len(runes) && unicode.IsDigit(runes[i]) && unicode.IsDigit(runes[i+n+1]) {
				n--
			}

			copy(runes[i+1:], runes[i+n+1:])
			runes = runes[:len(runes)-n]
		case unicode.IsLower(runes[i]) && !unicode.IsLower(runes[i+1]):
			// 小文字から小文字でない文字への変わり目
			endOfWord = true
		}
		i++

		// [start, i) が、ここまでに読んだ語
		text := string(runes[start:i])
		if !endOfWord {
			// 頭字語の直後が小文字でなければ、そこで語を分ける (IDFoo → ID・Foo)。
			// 小文字が続くときは分けない (URLs は 1 語)。
			if !initialisms[text] || unicode.IsLower(runes[i]) {
				if initialisms[text] {
					hasInitialism = true
				}

				continue
			}
		}

		isInitialism := false
		if upper := strings.ToUpper(text); initialisms[upper] {
			// 語の先頭 2 文字が ID・IP で、その 2 つ後ろが大文字のときは、頭字語として分けない
			// (IDENTITIES を ID・ENTITIES に、IPHONE を IP・HONE に分けない)。
			if upper == "ID" || upper == "IP" {
				if rest := runes[start:]; text == string(rest[:2]) && !endOfWord && len(rest) > 3 && unicode.IsUpper(rest[3]) {
					continue
				}
			}
			hasInitialism = true
			isInitialism = true
		}

		f(word{text: text, isInitialism: isInitialism, hasInitialism: hasInitialism})
		hasInitialism = false
		start = i
	}
}

// isDelimiter は、c が語の区切り (_・-・空白) かを返す。
func isDelimiter(c rune) bool {
	return c == '-' || c == '_' || unicode.IsSpace(c)
}

// upperFirst は、s の先頭の文字を大文字にする。
func upperFirst(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}
