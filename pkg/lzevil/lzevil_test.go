package lzevil

import (
	"bytes"
	"testing"
)

var testData = []byte("abcdabcdabcdabcd")

var testPackedData = []byte{
	0x10, 0x00, 0x00, 0x00,
	0x30, 0x61, 0x62, 0x63, 0x64, 0x83, 0x01,
}

func TestPacking(t *testing.T) {
	if packed := Compress(testData); !bytes.Equal(packed, testPackedData) {
		t.Fail()
	}
}

func TestUnpacking(t *testing.T) {
	if unpacked, err := Decompress(testPackedData); err != nil || !bytes.Equal(unpacked, testData) {
		t.Fail()
	}
}

func rand(seed *int) int {
	*seed = (*seed)*1103515245 + 12345
	return ((*seed) >> 0x10) & 0x7FFF
}

func TestPackingConsistency(t *testing.T) {
	bigData := make([]byte, 0x30000)
	seed := 0
	for i := range bigData {
		badRand := byte(rand(&seed) % 200)
		if badRand > 5 {
			badRand = 0
		}
		bigData[i] = badRand
	}

	testsStrings := []string{"", "1", "11", "123123123", "123123123x", "1123xxxxx3211", string(bigData)}
	for _, str := range testsStrings {
		data := []byte(str)
		packed := Compress(data)
		unpacked, err := Decompress(packed)
		if err != nil || !bytes.Equal(unpacked, data) {
			t.FailNow()
		}
	}
}
