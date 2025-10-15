package graphqljson

import (
	"fmt"

	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// UnmarshalData parses the GraphQL response payload contained in data and stores
// the result into v, which must be a non-nil pointer.
func UnmarshalData(data jsontext.Value, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode graphql data: decode json: %w", err)
	}

	return nil
}
