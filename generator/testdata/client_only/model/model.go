package domain

type Status string

const (
	StatusOpen   Status = "OPEN"
	StatusClosed Status = "CLOSED"
)

type Item struct {
	ID     string
	Status Status
	Name   string
}
