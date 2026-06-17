// Package cmputil はテストの比較ヘルパーを提供する。
package cmputil

// EqualErrorMessage は2つの error を Error() の文字列で等価判定する。
// 期待エラーと実エラーを文字列で比較するための cmp.Comparer として使う
// (cmp.Diff(want, got, cmp.Comparer(cmputil.EqualErrorMessage)))。両方 nil は等価。
func EqualErrorMessage(x, y error) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}

	return x.Error() == y.Error()
}
