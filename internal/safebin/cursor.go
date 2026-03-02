package safebin

import "encoding/binary"

type Cursor struct {
	data []byte
	pos  int
}

func NewCursor(data []byte) *Cursor {
	return &Cursor{data: data}
}

func (c *Cursor) Pos() int {
	return c.pos
}

func (c *Cursor) Remaining() int {
	return len(c.data) - c.pos
}

func (c *Cursor) Seek(offset int) error {
	if offset < 0 || offset > len(c.data) {
		return &ParseError{
			Op:     "seek",
			Offset: offset,
			Need:   0,
			Have:   len(c.data),
		}
	}
	c.pos = offset
	return nil
}

func (c *Cursor) Slice(offset, n int) ([]byte, error) {
	if offset < 0 || n < 0 || offset > len(c.data)-n {
		return nil, &ParseError{
			Op:     "slice",
			Offset: offset,
			Need:   n,
			Have:   len(c.data) - offset,
		}
	}
	return c.data[offset : offset+n], nil
}

func (c *Cursor) ReadBytes(n int) ([]byte, error) {
	start := c.pos
	out, err := c.Slice(start, n)
	if err != nil {
		return nil, &ParseError{
			Op:     "read-bytes",
			Offset: start,
			Need:   n,
			Have:   len(c.data) - start,
		}
	}
	c.pos += n
	return out, nil
}

func (c *Cursor) ReadU8() (byte, error) {
	b, err := c.ReadBytes(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (c *Cursor) ReadU16BE() (uint16, error) {
	b, err := c.ReadBytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (c *Cursor) ReadU32BE() (uint32, error) {
	b, err := c.ReadBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (c *Cursor) ReadU64BE() (uint64, error) {
	b, err := c.ReadBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}
