package packet

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"

	"github.com/juju/errors"
	. "github.com/siddontang/go-mysql/mysql"
)

/*
	Conn is the base class to handle MySQL protocol.
*/
type Conn struct {
	net.Conn
	br *bufio.Reader

	Sequence uint8
}

func NewConn(conn net.Conn) *Conn {
	c := new(Conn)

	c.br = bufio.NewReaderSize(conn, 4096)
	c.Conn = conn

	return c
}

func (c *Conn) ReadPacket() ([]byte, error) {
	var buf bytes.Buffer

	if err := c.ReadPacketTo(&buf); err != nil {
		return nil, errors.Trace(err)
	} else {
		return buf.Bytes(), nil
	}

	// header := []byte{0, 0, 0, 0}

	// if _, err := io.ReadFull(c.br, header); err != nil {
	// 	return nil, ErrBadConn
	// }

	// length := int(uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16)
	// if length < 1 {
	// 	return nil, fmt.Errorf("invalid payload length %d", length)
	// }

	// sequence := uint8(header[3])

	// if sequence != c.Sequence {
	// 	return nil, fmt.Errorf("invalid sequence %d != %d", sequence, c.Sequence)
	// }

	// c.Sequence++

	// data := make([]byte, length)
	// if _, err := io.ReadFull(c.br, data); err != nil {
	// 	return nil, ErrBadConn
	// } else {
	// 	if length < MaxPayloadLen {
	// 		return data, nil
	// 	}

	// 	var buf []byte
	// 	buf, err = c.ReadPacket()
	// 	if err != nil {
	// 		return nil, ErrBadConn
	// 	} else {
	// 		return append(data, buf...), nil
	// 	}
	// }
}

func (c *Conn) ReadPacketTo(w io.Writer) error {
	header := []byte{0, 0, 0, 0}

	if _, err := io.ReadFull(c.br, header); err != nil {
		return ConnError{
			Cause:   errors.Trace(err),
			Message: "Failed to read packet header",
		}
	}

	length := int(uint32(header[0]) | uint32(header[1])<<8 | uint32(header[2])<<16)
	if length < 1 {
		return errors.Errorf("invalid payload length %d", length)
	}

	sequence := uint8(header[3])

	if sequence != c.Sequence {
		return errors.Errorf("invalid sequence %d != %d", sequence, c.Sequence)
	}

	c.Sequence++

	if n, err := io.CopyN(w, c.br, int64(length)); err != nil {
		return ConnError{
			Cause:   errors.Trace(err),
			Message: "Failed to read packet body",
		}
	} else if n != int64(length) {
		return ConnError{
			Cause:   errors.Trace(err),
			Message: fmt.Sprintf("Packet body length mismatch. Expected %d, got %d", length, n),
		}
	} else {
		if length < MaxPayloadLen {
			return nil
		}

		if err := c.ReadPacketTo(w); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// data already has 4 bytes header
// will modify data inplace
func (c *Conn) WritePacket(data []byte) error {
	length := len(data) - 4

	for length >= MaxPayloadLen {
		data[0] = 0xff
		data[1] = 0xff
		data[2] = 0xff

		data[3] = c.Sequence

		bodyOffset := 4 + MaxPayloadLen
		if n, err := c.Write(data[:bodyOffset]); err != nil {
			return ConnError{
				Cause:   errors.Trace(err),
				Message: "Failed to write packet header",
			}
		} else if n != bodyOffset {
			msg := fmt.Sprintf("Packet header not fully written. Size: %d, written %d", bodyOffset, n)
			return errors.Trace(ConnError{Message: msg})
		} else {
			c.Sequence++
			length -= MaxPayloadLen
			data = data[MaxPayloadLen:]
		}
	}

	data[0] = byte(length)
	data[1] = byte(length >> 8)
	data[2] = byte(length >> 16)
	data[3] = c.Sequence

	if n, err := c.Write(data); err != nil {
		return ConnError{
			Cause:   errors.Trace(err),
			Message: "Failed to write packet body",
		}
	} else if n != len(data) {
		msg := fmt.Sprintf("Packet body not fully written. Size: %d, written %d", len(data), n)
		return errors.Trace(ConnError{Message: msg})
	} else {
		c.Sequence++
		return nil
	}
}

func (c *Conn) ResetSequence() {
	c.Sequence = 0
}

func (c *Conn) Close() error {
	c.Sequence = 0
	if c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
}

// ConnError contains connection error details.
type ConnError struct {
	Cause   error
	Message string
}

// Error implements error interface.
func (e ConnError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s. Cause: %s", e.Message, e.Cause.Error())
	}

	return e.Message
}

func IsConnError(err error) bool {
	if err == nil {
		return false
	}

	cause := errors.Cause(err)
	_, ok := cause.(ConnError)
	return ok
}
