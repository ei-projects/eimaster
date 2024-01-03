package lzevil

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const (
	windowBits  = 10
	windowSize  = 1 << windowBits
	windowMask  = 1<<windowBits - 1
	maxDataSize = math.MaxInt32
	hashBits    = 10
	hashSize    = 1 << hashBits
	hashMask    = 1<<hashBits - 1
)

var blockSizeCoder = newHuffmanCoder([]symbol{
	{base: 1, extraBitsLen: 0, bitsLen: 1, bits: 0},   //      0
	{base: 2, extraBitsLen: 0, bitsLen: 3, bits: 1},   //    001
	{base: 4, extraBitsLen: 0, bitsLen: 3, bits: 5},   //    101
	{base: 7, extraBitsLen: 2, bitsLen: 4, bits: 11},  //   1011
	{base: 3, extraBitsLen: 0, bitsLen: 5, bits: 3},   //  00011
	{base: 11, extraBitsLen: 3, bitsLen: 5, bits: 19}, //  10011
	{base: 5, extraBitsLen: 0, bitsLen: 5, bits: 7},   //  00111
	{base: 35, extraBitsLen: 6, bitsLen: 5, bits: 23}, //  10111
	{base: 6, extraBitsLen: 0, bitsLen: 5, bits: 15},  //  01111
	{base: 19, extraBitsLen: 4, bitsLen: 6, bits: 31}, // 011111
	{base: 99, extraBitsLen: 7, bitsLen: 6, bits: 63}, // 111111
})

var distCoder = newHuffmanCoder([]symbol{
	{base: 354, extraBitsLen: 10, bitsLen: 2, bits: 3},  //    11
	{base: 1378, extraBitsLen: 12, bitsLen: 2, bits: 1}, //    01
	{base: 98, extraBitsLen: 8, bitsLen: 3, bits: 6},    //   110
	{base: 34, extraBitsLen: 6, bitsLen: 3, bits: 2},    //   010
	{base: 6, extraBitsLen: 2, bitsLen: 4, bits: 4},     //  0100
	{base: 2, extraBitsLen: 2, bitsLen: 4, bits: 8},     //  1000
	{base: 10, extraBitsLen: 3, bitsLen: 4, bits: 0},    //  0000
	{base: 0, extraBitsLen: 1, bitsLen: 5, bits: 28},    // 11100
	{base: 18, extraBitsLen: 4, bitsLen: 5, bits: 12},   // 01100
})

type lzWriter struct {
	err          error
	writer       *bufio.Writer
	bw           *bitsWriter
	originalSize int32 // Size of uncompressed data
	writedSize   int32
	window       []byte
	windowPos    int
	prevByte     byte
	blockPos     int
	blockSize    int
	hashHead     [hashSize]int
	hashPrev     [windowSize]int
}

func NewWriter(w io.Writer, size int) io.Writer {
	if size < 0 || size > maxDataSize {
		panic("Invalid size")
	}

	return &lzWriter{
		writer:       bufio.NewWriter(w),
		originalSize: int32(size),
		window:       make([]byte, windowSize*2),
		blockPos:     -1,
	}
}

func (lzw *lzWriter) init() bool {
	lzw.err = binary.Write(lzw.writer, binary.LittleEndian, lzw.originalSize)
	lzw.bw = newBitsWriter(lzw.writer)
	return lzw.err == nil
}

func (lzw *lzWriter) writeBlock(pos int) {
	blockSizeCoder.encodeValue(uint32(lzw.blockSize), lzw.bw)
	distCoder.encodeValue(uint32(pos-lzw.blockPos-lzw.blockSize-1)&windowMask, lzw.bw)
	lzw.blockPos = -1
}

func (lzw *lzWriter) writeByte() {
	lzw.bw.writeBit(0) // Optimized blockSizeCoder.encodeValue(1, lzw.bw)
	lzw.bw.writeByte(lzw.prevByte)
	lzw.blockPos = -1
}

func (lzw *lzWriter) finishEncoding() {
	if lzw.blockPos >= 0 {
		lzw.writeBlock(lzw.windowPos + 1)
	} else {
		lzw.writeByte()
	}
	if lzw.err == nil {
		lzw.err = lzw.bw.flush()
	}
}

func hash10(a, b byte) int {
	return int((a << 2) ^ b)
}

func (lzw *lzWriter) Write(p []byte) (int, error) {
	if lzw.err != nil || lzw.bw == nil && !lzw.init() {
		return 0, lzw.err
	}

	data, remainingSize := p, int(lzw.originalSize-lzw.writedSize)
	if len(p) > remainingSize {
		data = p[:remainingSize]
	}

	startOfs := 0
	if lzw.writedSize == 0 && len(data) > 0 {
		lzw.hashPrev[0] = windowSize
		lzw.windowPos = 1
		lzw.window[lzw.windowPos] = data[0]
		lzw.window[lzw.windowPos+windowSize] = data[0]
		lzw.prevByte = data[0]
		startOfs = 1
	}

	maxBlockSize := int(blockSizeCoder.maxValue)
	for _, b := range data[startOfs:] {
		if lzw.err != nil {
			return 0, lzw.err
		}
		if lzw.err = lzw.bw.flushBuffer(false); lzw.err != nil {
			return 0, lzw.err
		}

		hash := hash10(lzw.prevByte, b)
		dist := (lzw.windowPos - lzw.hashHead[hash]) & windowMask
		if dist == 0 {
			dist = windowSize
		}
		lzw.hashHead[hash] = lzw.windowPos
		lzw.hashPrev[lzw.windowPos] = dist

		lzw.windowPos = (lzw.windowPos + 1) & windowMask
		lzw.window[lzw.windowPos] = b
		lzw.window[lzw.windowPos+windowSize] = b

		if lzw.blockPos < 0 {
			testBytes := []byte{lzw.prevByte, b}
			for dist < windowSize-maxBlockSize-1 {
				testPos := (lzw.windowPos - dist - 1) & windowMask
				if bytes.Equal(testBytes, lzw.window[testPos:testPos+2]) {
					lzw.blockPos = testPos
					lzw.blockSize = 2
					goto blockFound
				}
				dist += lzw.hashPrev[testPos]
			}
			lzw.writeByte()
		} else if lzw.blockSize == maxBlockSize {
			lzw.writeBlock(lzw.windowPos)
		} else if lzw.window[lzw.blockPos+lzw.blockSize] == b {
			lzw.blockSize++
		} else {
			relPos := lzw.windowPos - lzw.blockSize
			dist2 := (relPos - lzw.blockPos + lzw.hashPrev[lzw.blockPos]) & windowMask
			testBytes := lzw.window[relPos&windowMask : relPos&windowMask+lzw.blockSize+1]
			for dist2 < windowSize-maxBlockSize-1 {
				for dist2 > dist {
					dist += lzw.hashPrev[(lzw.windowPos-dist-1)&windowMask]
				}
				if dist2 < dist {
					dist2 += lzw.hashPrev[(relPos-dist2)&windowMask]
				} else {
					testPos := (relPos - dist2) & windowMask
					if bytes.Equal(testBytes, lzw.window[testPos:testPos+lzw.blockSize+1]) {
						lzw.blockPos = testPos
						lzw.blockSize++
						goto blockFound
					}
					dist2 += lzw.hashPrev[testPos]
					dist += lzw.hashPrev[(lzw.windowPos-dist-1)&windowMask]
				}
			}
			lzw.writeBlock(lzw.windowPos)
		}

	blockFound:
		lzw.prevByte = b
	}

	lzw.writedSize += int32(len(data))
	if lzw.writedSize == lzw.originalSize {
		lzw.finishEncoding()
		if lzw.err == nil {
			lzw.err = lzw.writer.Flush()
		}
		if lzw.err == nil {
			lzw.err = io.EOF
		}
	}

	return len(data), lzw.err
}

type lzReader struct {
	err          error
	reader       io.Reader
	br           *bitsReader
	buf          bytes.Buffer
	originalSize int32 // Size of uncompressed data
	readedSize   int32
	window       [windowSize]byte
	windowPos    uint32
}

var ErrInvalidData = errors.New("Invalid data")

func NewReader(r io.Reader) io.Reader {
	return &lzReader{reader: bufio.NewReader(r)}
}

func (lzr *lzReader) init() bool {
	lzr.err = binary.Read(lzr.reader, binary.LittleEndian, &lzr.originalSize)
	lzr.br = newBitsReader(lzr.reader)
	if errors.Is(lzr.err, io.EOF) {
		lzr.err = io.ErrUnexpectedEOF
	}
	if lzr.err == nil && lzr.originalSize < 0 {
		lzr.err = ErrInvalidData
	}
	return lzr.err == nil
}

func (lzr *lzReader) Read(p []byte) (int, error) {
	if lzr.err != nil || lzr.br == nil && !lzr.init() {
		return 0, lzr.err
	}

	for lzr.readedSize < lzr.originalSize && lzr.buf.Len() < len(p) {
		var blockSize, dist uint32
		var b byte
		blockSize = blockSizeCoder.decodeValue(lzr.br)
		if blockSize > 1 {
			dist = distCoder.decodeValue(lzr.br)
			for i := uint32(0); i < blockSize; i++ {
				pos := (lzr.windowPos - dist - 1) & windowMask
				b = lzr.window[pos]
				lzr.buf.WriteByte(b)
				lzr.window[lzr.windowPos] = b
				lzr.windowPos = (lzr.windowPos + 1) & windowMask
			}
		} else {
			b = lzr.br.readByte()
			lzr.buf.WriteByte(b)
			lzr.window[lzr.windowPos] = b
			lzr.windowPos = (lzr.windowPos + 1) & windowMask
		}
		if lzr.br.err != nil {
			return 0, lzr.br.err
		}

		lzr.readedSize += int32(blockSize)
	}

	n, err := lzr.buf.Read(p)
	data := lzr.buf.Next(lzr.buf.Len())
	lzr.buf.Reset()
	lzr.buf.Write(data)
	return n, err
}

func Compress(data []byte) []byte {
	var buf bytes.Buffer
	if n, err := NewWriter(&buf, len(data)).Write(data); n != len(data) || !errors.Is(err, io.EOF) {
		panic("Unexpected error")
	}
	return buf.Bytes()
}

func Decompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(NewReader(bytes.NewReader(data)))
	return buf.Bytes(), err
}
