package transport

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewHeader(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	// header(ctx) はリクエストごとに評価されるため、ctx 由来の動的な値を付与できる
	wrap := NewHeader(func(ctx context.Context) http.Header {
		token, _ := ctx.Value(ctxKey{}).(string)
		return http.Header{"Authorization": []string{"Bearer " + token}}
	})

	var got http.Header
	base := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		got = req.Header
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
	})

	ctx := context.WithValue(t.Context(), ctxKey{}, "tok-1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set("X-Existing", "keep")

	if _, err := wrap(base).RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	// ctx の値で動的ヘッダーが付与され、既存ヘッダーも保持される
	want := http.Header{
		"X-Existing":    []string{"keep"},
		"Authorization": []string{"Bearer tok-1"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("header diff(-want +got): %s", diff)
	}

	// RoundTripper は元のリクエストを変更しない (clone している)
	if req.Header.Get("Authorization") != "" {
		t.Errorf("original request was mutated: %v", req.Header)
	}
}
