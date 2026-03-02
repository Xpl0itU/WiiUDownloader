package safebin

import "fmt"

type ParseError struct {
	Op     string
	Offset int
	Need   int
	Have   int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s failed at offset %d (need %d bytes, have %d)", e.Op, e.Offset, e.Need, e.Have)
}
