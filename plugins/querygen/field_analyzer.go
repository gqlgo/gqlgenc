package querygen

import (
	"go/types"

	"github.com/99designs/gqlgen/codegen/templates"
)

// FieldAnalyzer はGo構造体のフィールドを解析し、FieldInfo構造体のリストを構築する。
// 埋め込みフィールドの再帰的解析、インラインフラグメントの検出、
// JSONタグの解析などを行い、GraphQLクエリの型情報を抽出する。
type FieldAnalyzer struct {
	classifier *FieldClassifier
}

// NewFieldAnalyzer creates a new FieldAnalyzer
func NewFieldAnalyzer() *FieldAnalyzer {
	return &FieldAnalyzer{
		classifier: NewFieldClassifier(),
	}
}

// AnalyzeFields は全フィールドを解析する
func (a *FieldAnalyzer) AnalyzeFields(
	structType *types.Struct,
	shouldGenerateUnmarshal func(*types.Named) bool,
) []FieldInfo {
	fields := make([]FieldInfo, 0, structType.NumFields())

	for i := range structType.NumFields() {
		field := structType.Field(i)
		tag := structType.Tag(i)

		info := a.analyzeField(field, tag, shouldGenerateUnmarshal)
		fields = append(fields, info)
	}

	return fields
}

// analyzeField は1つのフィールドを解析して FieldInfo を返す
func (a *FieldAnalyzer) analyzeField(
	field *types.Var,
	tag string,
	shouldGenerateUnmarshal func(*types.Named) bool,
) FieldInfo {
	info := FieldInfo{
		Name:       field.Name(),
		Type:       field.Type(),
		TypeName:   templates.CurrentImports.LookupType(field.Type()),
		JSONTag:    a.classifier.parseJSONTag(tag),
		IsExported: field.Exported(),
		IsEmbedded: field.Anonymous(),
	}

	if a.classifier.IsInlineFragment(field, tag) {
		info.IsInlineFragment = true

		if ptrType, ok := field.Type().(*types.Pointer); ok {
			info.IsPointer = true
			elemType := ptrType.Elem()
			info.PointerElemType = templates.CurrentImports.LookupType(elemType)
		}
	}

	// 埋め込みフィールドでインラインフラグメントでない場合の特別処理
	// GraphQLのフラグメントスプレッドに対応するため、埋め込みフィールドは
	// 独自のUnmarshalJSONメソッドを持つ場合と、親の型に展開される場合がある
	if info.IsEmbedded && !info.IsInlineFragment {
		if embeddedNamed := namedStructType(field.Type()); embeddedNamed != nil {
			// 埋め込み型が独自のUnmarshalJSONを持つ場合は、サブフィールドを解析せず早期リターン
			if shouldGenerateUnmarshal(embeddedNamed) {
				return info
			}
		}

		// 埋め込みフィールドのサブフィールドを再帰的に解析
		// これにより、ネストした埋め込み構造もフラット化される
		if embeddedStruct := getStructType(field.Type()); embeddedStruct != nil {
			info.SubFields = a.AnalyzeFields(embeddedStruct, shouldGenerateUnmarshal)
		}
	}

	return info
}
