package eimasterlib

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"golang.org/x/text/encoding/charmap"
)

func unexpectEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

func checkErr(err *error, err2 error) {
	if *err == nil {
		*err = err2
	}
}

func checkErrNoEOF(err *error, err2 error) {
	if *err == nil {
		*err = unexpectEOF(err2)
	}
}

func readFull(r io.Reader, buf []byte) error {
	if n, err := io.ReadFull(r, buf); n != len(buf) {
		return err
	}
	return nil
}

func readLE(r io.Reader, data interface{}) error {
	return binary.Read(r, binary.LittleEndian, data)
}

func writeLE(w io.Writer, data interface{}) error {
	return binary.Write(w, binary.LittleEndian, data)
}

func readByte(r io.Reader, val *byte) error {
	var buf [1]byte
	if err := readFull(r, buf[:]); err != nil {
		return err
	}
	*val = buf[0]
	return nil
}

func writeIntAsUint8(w io.Writer, val int) error {
	if val < 0 || val > math.MaxUint8 {
		return errors.New("Value does not fit")
	}
	return writeLE(w, uint8(val))
}

func writeIntAsInt8(w io.Writer, val int) error {
	if val < math.MinInt8 || val > math.MaxInt8 {
		return errors.New("Value does not fit")
	}
	return writeLE(w, int8(val))
}

func writeIntAsUint32(w io.Writer, val int) error {
	if val < 0 || val > math.MaxUint32 {
		return errors.New("Value does not fit")
	}
	return writeLE(w, uint32(val))
}

func writeIntAsInt32(w io.Writer, val int) error {
	if val < math.MinInt32 || val > math.MaxInt32 {
		return errors.New("Value does not fit")
	}
	return writeLE(w, int32(val))
}

func DecodeWin1251(encoded []byte) string {
	dec := charmap.Windows1251.NewDecoder()
	out, _ := dec.Bytes(encoded)
	return string(out)
}

func EncodeWin1251(decoded string) []byte {
	enc := charmap.Windows1251.NewEncoder()
	out, _ := enc.String(decoded)
	return []byte(out)
}
