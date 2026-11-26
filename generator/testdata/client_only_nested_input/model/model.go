package model

// Inner is not listed in models:, only Outer is.
type Inner struct {
	ID *string `json:"id,omitempty"`
}

type Outer struct {
	Inner *Inner `json:"inner,omitempty"`
}
