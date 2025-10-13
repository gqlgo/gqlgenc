package graphqljson

import (
	"reflect"
	"strings"
)

type structInfo struct {
	fields        []structFieldInfo
	fallbackIndex int
}

type structFieldInfo struct {
	index          int
	name           string
	graphqlName    string
	hasGraphQLName bool
	isFragment     bool
	fragmentType   string
	anonymous      bool
	jsonUnknown    bool
	jsonName       string
	omit           bool
}

func buildStructInfo(t reflect.Type) *structInfo {
	info := &structInfo{fallbackIndex: -1}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}

		sf := structFieldInfo{index: i, name: f.Name, anonymous: f.Anonymous}

		jsonName, jsonOpts := parseJSONTag(f.Tag.Get("json"))
		if jsonName == "-" {
			sf.omit = true
			info.fields = append(info.fields, sf)
			continue
		}
		sf.jsonName = jsonName
		for _, opt := range jsonOpts {
			if opt == "unknown" {
				sf.jsonUnknown = true
			}
		}

		if gqlName, fragmentType, isFragment, ok := parseGraphQLTag(f.Tag.Get("graphql")); ok {
			sf.isFragment = isFragment
			if gqlName != "" {
				sf.graphqlName = gqlName
				sf.hasGraphQLName = true
			}
			sf.fragmentType = fragmentType
		}

		if sf.jsonUnknown && sf.jsonName == "" && !sf.isFragment {
			info.fallbackIndex = len(info.fields)
		}

		info.fields = append(info.fields, sf)
	}

	return info
}

func parseJSONTag(tag string) (name string, opts []string) {
	if tag == "" {
		return "", nil
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if len(parts) > 1 {
		opts = parts[1:]
	}
	return name, opts
}

func parseGraphQLTag(tag string) (name string, fragmentType string, isFragment bool, ok bool) {
	if tag == "" {
		return "", "", false, false
	}
	val := strings.TrimSpace(tag)
	if strings.HasPrefix(val, "...") {
		fragmentType = parseFragmentType(val)
		return "", fragmentType, true, true
	}
	if idx := strings.IndexAny(val, "(:@"); idx != -1 {
		val = val[:idx]
	}
	return strings.TrimSpace(val), "", false, true
}

func parseFragmentType(tag string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(tag, "..."))
	if strings.HasPrefix(rest, "on") {
		rest = strings.TrimSpace(rest[len("on"):])
	}
	for i, r := range rest {
		if r == ' ' || r == '{' || r == '(' || r == '@' || r == ':' {
			return strings.TrimSpace(rest[:i])
		}
	}
	return strings.TrimSpace(rest)
}
