package lzevil

import (
	"bytes"
	"testing"
)

func TestCoder(t *testing.T) {
	tables := [][]symbol{
		{{0, 0, 1, 0}, {1, 1, 1, 0}},
		{{0, 0, 1, 2}, {4, 1, 1, 3}},
	}
	var buf bytes.Buffer
	for _, symbols := range tables {
		coder := newHuffmanCoder(symbols)
		for _, sym := range symbols {
			for extra := uint32(0); extra < 1<<sym.extraBitsLen; extra++ {
				buf.Reset()
				val := sym.base + extra
				bw := newBitsWriter(&buf)
				coder.encodeValue(val, bw)
				bw.flush()
				br := newBitsReader(&buf)
				val2 := coder.decodeValue(br)
				if val != val2 || br.err != nil {
					t.FailNow()
				}
			}
		}
	}
}
