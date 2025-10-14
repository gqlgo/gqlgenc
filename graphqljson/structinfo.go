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
	index       int
	name        string
	jsonName    string
	anonymous   bool
	jsonUnknown bool
	omit        bool
}

func buildStructInfo(t reflect.Type) *structInfo {
	info := &structInfo{fallbackIndex: -1}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		// Skip unexported fields, but allow anonymous embedded fields
		// even if they have a non-empty PkgPath (which indicates the package
		// where the embedded type is defined, not that the field is unexported)
		if f.PkgPath != "" && !f.Anonymous {
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

		if sf.jsonUnknown && sf.jsonName == "" {
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
