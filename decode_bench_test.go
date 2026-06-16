package main

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
	"testing"

	ifudomain "github.com/Yamashou/gqlgenc/v3/testdata/integration/interfaceunion/domain"
)

// flatCell mirrors the union's decodable fields as a single flat struct decoded
// in one json/v2 pass. It is the baseline against the generated Search_Search,
// whose UnmarshalJSONFrom buffers the value and re-tokenizes it once for the
// plain pass and once more for the matched inline fragment.
type flatCell struct {
	Typename *string            `json:"__typename"`
	ID       string             `json:"id"`
	Title    string             `json:"title"`
	Name     string             `json:"name"`
	Kind     ifudomain.NodeKind `json:"kind"`
}

func buildCellsJSON(n int) []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		if i%2 == 0 {
			b.WriteString(`{"__typename":"User","id":"u","name":"Alice","kind":"USER"}`)
		} else {
			b.WriteString(`{"__typename":"Post","id":"p","title":"Hello"}`)
		}
	}
	b.WriteByte(']')
	return []byte(b.String())
}

// BenchmarkDecodeUnion compares decoding a list of union elements through the
// generated UnmarshalJSONFrom (N-pass) against decoding the same JSON into an
// equivalent flat struct via json/v2's default single-pass decode.
func BenchmarkDecodeUnion(b *testing.B) {
	for _, n := range []int{100, 1000} {
		data := buildCellsJSON(n)

		b.Run(fmt.Sprintf("generated_union/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for range b.N {
				var got []*ifudomain.Search_Search
				if err := json.Unmarshal(data, &got); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("flat_baseline/n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for range b.N {
				var got []flatCell
				if err := json.Unmarshal(data, &got); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
