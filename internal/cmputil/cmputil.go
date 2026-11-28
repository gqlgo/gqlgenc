// Package cmputil はテストの比較ヘルパーを提供する。
package cmputil

// EqualErrorMessage は2つの error を Error() の文字列で等価判定する。両方 nil は等価。
//
// トップレベルの裸の error 比較では真偽値ヘルパーとして使う
// (if !cmputil.EqualErrorMessage(want, got) { ... })。期待値と実値の具象型が
// 異なると cmp はルートのノード型を interface{} に潰し、interface{} は error に
// 代入可能でないため cmp.Comparer 形 (cmp.Diff(want, got, cmp.Comparer(...))) は
// スキップされてしまう (cmpopts.EquateErrors が Comparer ではなく FilterValues で
// 実装されているのも同じ理由)。Comparer 形にしたい場合は EquateErrors と同様に
// cmp.FilterValues でラップする。struct のフィールド (静的型が error) の比較なら
// Comparer 形でも発火する。
func EqualErrorMessage(x, y error) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}

	return x.Error() == y.Error()
}
