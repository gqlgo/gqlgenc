//go:build gqlgenc_never_built

// このファイルは、どのビルドにも含めない。束縛先のパッケージが「ビルド制約ですべてのファイルが
// 外れる」読み込みエラーになる状況を、コンパイルできないコードを置かずに再現するためである。

package domain

type Status string
