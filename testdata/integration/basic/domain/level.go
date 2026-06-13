package domain

import (
	"fmt"
	"strconv"
)

// Level is an int-based enum bound to the GraphQL Level enum via the
// @goModel / @goEnum directives (https://gqlgen.com/recipes/enum/).
//
// The wire format of a GraphQL enum is its name string (e.g. "HIGH"),
// so an int-based bound type must implement json.Marshaler and
// json.Unmarshaler to map between names and values on the client side
// (gqlgo/gqlgenc#229).
type Level int

const (
	LevelLow Level = iota + 1
	LevelHigh
)

// MarshalJSON implements json.Marshaler interface
func (l Level) MarshalJSON() ([]byte, error) {
	switch l {
	case LevelLow:
		return []byte(`"LOW"`), nil
	case LevelHigh:
		return []byte(`"HIGH"`), nil
	}
	return nil, fmt.Errorf("invalid Level: %d", l)
}

// UnmarshalJSON implements json.Unmarshaler interface
func (l *Level) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return fmt.Errorf("level must be a string: %w", err)
	}

	switch s {
	case "LOW":
		*l = LevelLow
	case "HIGH":
		*l = LevelHigh
	default:
		return fmt.Errorf("invalid Level: %q", s)
	}

	return nil
}
