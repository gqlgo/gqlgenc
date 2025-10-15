package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Email represents an email address as a custom scalar type
type Email string

// UnmarshalJSON implements json.Unmarshaler interface
func (e *Email) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	// Simple validation: check if the email contains '@'
	if !strings.Contains(s, "@") {
		return fmt.Errorf("invalid email format: %s", s)
	}

	*e = Email(s)
	return nil
}

// MarshalJSON implements json.Marshaler interface
func (e Email) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(e))
}

// String returns the string representation of the email
func (e Email) String() string {
	return string(e)
}
