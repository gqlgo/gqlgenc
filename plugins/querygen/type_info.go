package querygen

// FieldInfo は構造体フィールドの情報を表す。
//
// この構造体は各フィールドのメタデータを保持し、適切なアンマーシャル
// ロジックとコード生成を可能にする。
type FieldInfo struct {
	Name             string      // フィールド名
	TypeName         string      // インポート修飾された型名
	JSONTag          string      // JSON タグの値（例: "id", "-"）
	IsExported       bool        // エクスポートされているか（先頭が大文字）
	IsEmbedded       bool        // 埋め込みフィールドか（匿名フィールド）
	IsInlineFragment bool        // inline fragment フィールドか
	IsPointer        bool        // ポインタ型か
	PointerElemType  string      // ポインタの要素型名（IsPointer が true の場合）
	SubFields        []FieldInfo // 埋め込みフィールドの場合、埋め込み構造体のフィールドを含む
}

// IsFragmentSpread は、フィールドが GraphQL の fragment spread（親の UnmarshalJSONFrom で
// デコードされる名前付き構造体フィールド）かどうかを返す。fragment spread と inline fragment は
// どちらも json:"-" を持つが、inline fragment はポインタ (IsInlineFragment) なので、
// それを除いた json:"-" の非ポインタフィールドを fragment spread とみなす。
func (f FieldInfo) IsFragmentSpread() bool {
	return (f.JSONTag == "" || f.JSONTag == "-") && !f.IsInlineFragment
}

// InlineFragmentInfo は inline fragment フィールドの情報を表す。
//
// Inline fragments は GraphQL の型条件付きフィールド（... on Type）を表し、
// __typename に基づいてアンマーシャルされる。
type InlineFragmentInfo struct {
	Field       FieldInfo // フィールド情報
	FieldExpr   string    // フィールド式（例: "t.User"）
	ElemTypeStr string    // 要素型の名前（例: "UserFragment"）
}
