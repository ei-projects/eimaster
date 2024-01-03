package lzevil

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReaderErr(t *testing.T) {
	tests := []struct {
		bitsLen     uint8
		dataSize    int
		expectedErr error
	}{
		{2, 0, io.EOF},
		{8, 1, io.EOF},
		{24, 2, io.ErrUnexpectedEOF},
	}
	for _, test := range tests {
		br := newBitsReader(bytes.NewReader(make([]byte, test.dataSize)))
		for br.err == nil {
			br.readBits(test.bitsLen)
		}
		if !errors.Is(br.err, test.expectedErr) {
			t.Errorf("For %d bits unexpected err is '%v'. Expected err is '%v'",
				test.bitsLen, br.err, test.expectedErr)
		}
	}
}

func TestBitsMax(t *testing.T) {
	vals := []uint32{0xFFFFFFFF, 0x81838587, 0xDEADBEEF, 0x12345678}
	var val1, val2, valOfs1, valOfs2 uint32
	var buf bytes.Buffer
	for _, val := range vals {
		for bitsCount := uint8(1); bitsCount <= 24; bitsCount++ {
			for bitsOfs := uint8(0); bitsOfs <= 24; bitsOfs++ {
				buf.Reset()
				bw := newBitsWriter(&buf)
				if bitsOfs > 0 {
					valOfs1 = val & (1<<bitsOfs - 1)
					bw.writeBits(valOfs1, bitsOfs)
				}
				val1 = val & (1<<bitsCount - 1)
				bw.writeBits(val1, bitsCount)
				bw.flush()

				br := newBitsReader(&buf)
				if bitsOfs > 0 {
					valOfs2 = br.readBits(bitsOfs)
					if valOfs1 != valOfs2 {
						t.FailNow()
					}
				}
				val2 = br.readBits(bitsCount)
				if val1 != val2 {
					t.FailNow()
				}
				if br.err != nil {
					t.FailNow()
				}
			}
		}
	}
}
