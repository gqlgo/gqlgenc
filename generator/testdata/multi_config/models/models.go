// Package models holds hand-written types bound with autobind by the
// multi_config fixture, standing in for a large domain package.
package models

// Address is bound to the Address type of the schema.
type Address struct {
	Street string `json:"street"`
	City   string `json:"city"`
}
