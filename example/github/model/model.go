package model

// Actor represents an object which can take actions on GitHub. Typically a User or Bot.
type Actor interface {
	IsActor()
	// A URL pointing to the actor's public avatar.
	GetAvatarURL() string
	// The username of the actor.
	GetLogin() string
	// The HTTP path for this actor.
	GetResourcePath() string
	// The HTTP URL for this actor.
	GetURL() string
}
