package domain

// AddressView is a hand-written type bound to the PublicAddressFields fragment
// via the @goFragment directive in the query, instead of a generated type.
type AddressView struct {
	ID     string `json:"id"`
	Street string `json:"street"`
	Public bool   `json:"public"`
}
