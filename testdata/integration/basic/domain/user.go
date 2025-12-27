package domain

type UserID string

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
