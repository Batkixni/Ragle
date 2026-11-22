package share

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DiscoveryPort is the UDP port every Ragle instance binds for presence.
const DiscoveryPort = 47321

const magic = "RGL1"

const maxFrame = 32 << 20

const (
	frameHello  byte = 'H'
	frameMeta   byte = 'M'
	frameAccept byte = 'A'
	frameFile   byte = 'F'
	frameEnd    byte = 'E'
)

// ErrDeclined means the receiver chose not to take the project.
var ErrDeclined = errors.New("ragle: declined")

// ErrClosed means the share node has stopped.
var ErrClosed = errors.New("ragle: share closed")

// ErrInterrupted means the connection ended before the transfer finished.
var ErrInterrupted = errors.New("ragle: transfer interrupted")

type hello struct {
	Name string `msgpack:"name"`
	ID   string `msgpack:"id"`
}

type fileHeader struct {
	Hash string `msgpack:"hash"`
	Name string `msgpack:"name"`
	Size int64  `msgpack:"size"`
}

func writeMagic(w io.Writer) error {
	_, err := io.WriteString(w, magic)
	return err
}

func readMagic(r io.Reader) error {
	buf := make([]byte, len(magic))
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	if string(buf) != magic {
		return errors.New("ragle: bad share magic")
	}
	return nil
}

func writeFrame(w io.Writer, kind byte, payload []byte) error {
	if len(payload) > maxFrame {
		return errors.New("ragle: frame too large")
	}
	var hdr [5]byte
	hdr[0] = kind
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > maxFrame {
		return 0, nil, errors.New("ragle: frame too large")
	}
	if n == 0 {
		return hdr[0], nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return hdr[0], buf, nil
}

func writeAccept(w io.Writer, yes bool) error {
	payload := []byte{'N'}
	if yes {
		payload = []byte{'Y'}
	}
	return writeFrame(w, frameAccept, payload)
}

func accepted(payload []byte) (bool, error) {
	if len(payload) != 1 || (payload[0] != 'Y' && payload[0] != 'N') {
		return false, fmt.Errorf("ragle: bad accept")
	}
	return payload[0] == 'Y', nil
}
