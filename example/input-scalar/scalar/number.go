package scalar

import (
	"fmt"
	"io"
	"strconv"
)

type Number int64

const (
	NumberOne Number = 1
	NumberTwo Number = 2
)

func (n *Number) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	switch str {
	case "ONE":
		*n = NumberOne
	case "TWO":
		*n = NumberTwo
	default:
		return fmt.Errorf("Number not found Type: %d", n)
	}

	return nil
}

func (n Number) MarshalGQL(w io.Writer) {
	var str string

	switch n {
	case NumberOne:
		str = "ONE"
	case NumberTwo:
		str = "TWO"
	}

	fmt.Fprint(w, strconv.Quote(str))
}
