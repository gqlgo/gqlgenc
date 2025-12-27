package domain

import (
	"fmt"
	"io"
	"strconv"
)

type UserID string

// UnmarshalGQL はGraphQLの値をDraftOrderIDに変換する。
func (u *UserID) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("%vは文字列である必要があります", v)
	}
	*u = UserID(str)
	return nil
}

// MarshalGQL はUserIDをGraphQLの値に変換する。
func (u UserID) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(string(u)))
}

type User struct {
	ID              UserID
	Name            string
	Email           Email
	Settings        *UserSettings
	Profile         Profile
	OptionalProfile Profile
	Address         Address
	OptionalAddress Address
	ProfilePic      string
}
