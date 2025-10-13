package graphqljson

import (
	"fmt"

	"encoding/json/jsontext"
	"encoding/json/v2"
)

func UnmarshalData(data jsontext.Value, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal graphql data: %w", err)
	}
	return nil
}
