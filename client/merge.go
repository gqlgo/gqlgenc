package client

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
)

// MergeJSONObject は v を JSON オブジェクトとしてエンコードし、そのメンバーを dst へ
// マージする。フラグメントを含むレスポンス型の MarshalJSONTo が、フラグメントの
// フィールド (json:"-" のためデフォルトマーシャルでは出力されない) を親と同じ
// オブジェクトへ平坦化して再出力するために使う。
//
// キーが重複した場合は GraphQL のセレクションマージに合わせて解決する。サーバーは
// 同じレスポンスキーへの選択を1つのオブジェクトへマージして返すため、親型と
// フラグメント型が同じキーの異なるサブフィールドを持ち得る。両方がオブジェクトなら
// 再帰的にマージ、両方が同じ長さの配列なら要素ごとにマージ、それ以外は既存値を維持する。
//
// ゼロ値のフィールドは省略されるため、出力はサーバーのレスポンスそのものではなく
// 「復元すると同じ Go 値になる JSON」である。
//
// 同じ入力からは毎回同じ JSON になる。v の中の map と、重複したキーのオブジェクトを再帰的に
// マージした結果は、メンバーをキーの昇順に並べる。
func MergeJSONObject(dst map[string]jsontext.Value, v any) error {
	// ゼロ値のフィールドは出力から省略する。レスポンスに含まれなかったフィールド
	// (union / interface メンバー向けフラグメントの非共有フィールドなど) はデコード後
	// ゼロ値になっており、そのまま再出力すると復元側のバリデーション (enum 等) で
	// 失敗する。省略すれば復元時に再びゼロ値へ戻るため、値としてのラウンドトリップ
	// (marshal → unmarshal で元の Go 値と一致) が成立する。
	//
	// json/v2 は、既定では map のメンバーを反復の順 (実行のたびに変わる) で出力する。
	// 同じ入力から毎回同じ JSON を得るため、json.Deterministic でキーの昇順にする。
	data, err := json.Marshal(v, json.OmitZeroStructFields(true), json.Deterministic(true))
	if err != nil {
		return fmt.Errorf("marshal for merge: %w", err)
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(data, &members); err != nil {
		return fmt.Errorf("merge source must be a JSON object: %w", err)
	}
	for key, value := range members {
		merged, err := mergeValue(dst[key], value)
		if err != nil {
			return fmt.Errorf("merge member %q: %w", key, err)
		}
		dst[key] = merged
	}
	return nil
}

func mergeValue(dst, src jsontext.Value) (jsontext.Value, error) {
	if dst == nil {
		return src, nil
	}
	if dst.Kind() == '{' && src.Kind() == '{' {
		var dstObj map[string]jsontext.Value
		if err := json.Unmarshal(dst, &dstObj); err != nil {
			return dst, fmt.Errorf("decode dst object: %w", err)
		}
		var srcObj map[string]jsontext.Value
		if err := json.Unmarshal(src, &srcObj); err != nil {
			return dst, fmt.Errorf("decode src object: %w", err)
		}
		for key, value := range srcObj {
			merged, err := mergeValue(dstObj[key], value)
			if err != nil {
				return dst, fmt.Errorf("merge object member %q: %w", key, err)
			}
			dstObj[key] = merged
		}
		// dstObj は map なので、キーの昇順で出力する (理由は MergeJSONObject のコメントを参照)
		data, err := json.Marshal(dstObj, json.Deterministic(true))
		if err != nil {
			return dst, fmt.Errorf("encode merged object: %w", err)
		}
		return data, nil
	}
	if dst.Kind() == '[' && src.Kind() == '[' {
		var dstArr []jsontext.Value
		if err := json.Unmarshal(dst, &dstArr); err != nil {
			return dst, fmt.Errorf("decode dst array: %w", err)
		}
		var srcArr []jsontext.Value
		if err := json.Unmarshal(src, &srcArr); err != nil {
			return dst, fmt.Errorf("decode src array: %w", err)
		}
		if len(dstArr) != len(srcArr) {
			return dst, nil
		}
		for i := range dstArr {
			merged, err := mergeValue(dstArr[i], srcArr[i])
			if err != nil {
				return dst, fmt.Errorf("merge array element %d: %w", i, err)
			}
			dstArr[i] = merged
		}
		data, err := json.Marshal(dstArr)
		if err != nil {
			return dst, fmt.Errorf("encode merged array: %w", err)
		}
		return data, nil
	}
	return dst, nil
}
