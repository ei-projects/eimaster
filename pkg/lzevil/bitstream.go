package lzevil

import "io"

type bitsReader struct {
	reader   io.Reader
	err      error
	bits     uint32
	bitsLen  uint8
	data     []byte
	dataPos  int
	dataSize int
}

func newBitsReader(r io.Reader) *bitsReader {
	return &bitsReader{
		reader: r,
		data:   make([]byte, 256),
	}
}

func (br *bitsReader) fetchBits() {
	br.bits |= uint32(br.data[br.dataPos]) << br.bitsLen
	br.bitsLen += 8
	br.dataPos++
	br.dataSize--
}

func (br *bitsReader) getBits(bitsLen uint8) uint32 {
	res := br.bits & (1<<bitsLen - 1)
	br.bits >>= bitsLen
	br.bitsLen -= bitsLen
	return res
}

func (br *bitsReader) fillBuffer(requiredBytes int) bool {
	if br.err != nil {
		return false
	}
	copy(br.data, br.data[br.dataPos:])
	n, err := io.ReadAtLeast(br.reader, br.data[br.dataSize:cap(br.data)], requiredBytes)
	br.dataSize += n
	br.dataPos = 0
	br.data = br.data[0:br.dataSize]
	br.err = err
	return n >= requiredBytes
}

func (br *bitsReader) readBit() uint32 {
	if br.bitsLen > 0 {
		return br.getBits(1)
	}
	if br.dataSize > 0 || br.fillBuffer(1) {
		br.fetchBits()
		return br.getBits(1)
	}
	return 0
}

func (br *bitsReader) readBits(bitsLen uint8) uint32 {
	if bitsLen < 1 || bitsLen > 24 {
		panic("Invalid bitsLen")
	}
	requiredBytes := int(bitsLen-br.bitsLen+7)/8 - br.dataSize
	if requiredBytes <= 0 || br.fillBuffer(requiredBytes) {
		for br.bitsLen < bitsLen {
			br.fetchBits()
		}
		return br.getBits(bitsLen)
	}
	return 0
}

func (br *bitsReader) readByte() byte {
	if br.dataSize > 0 || br.fillBuffer(1) {
		res := br.data[br.dataPos]
		br.dataPos++
		br.dataSize--
		return res
	}
	return 0
}

type bitsWriter struct {
	writer    io.Writer
	bits      uint32
	bitsLen   uint8
	bitsPos   int // Valid only if bitsLen > 0
	buf       []byte
	bufPos    int
	bufUpSize int
}

func newBitsWriter(w io.Writer) *bitsWriter {
	return &bitsWriter{
		writer:    w,
		buf:       make([]byte, 256),
		bufUpSize: 256 - 8,
	}
}

func (bw *bitsWriter) flushBuffer(force bool) error {
	dataLen := bw.bufPos
	if bw.bitsLen > 0 {
		dataLen = bw.bitsPos
	}
	if dataLen > len(bw.buf)-16 || dataLen > 0 && force {
		n, err := bw.writer.Write(bw.buf[:dataLen])
		if err != nil {
			return err
		}
		if n != dataLen {
			return io.ErrShortWrite
		}
		bw.bufPos -= dataLen
		bw.bitsPos = 0
		if bw.bufPos > 0 {
			copy(bw.buf, bw.buf[dataLen:dataLen+bw.bufPos])
		}
	}
	return nil
}

func (bw *bitsWriter) flush() error {
	if bw.bitsLen > 0 {
		bw.buf[bw.bitsPos] = byte(bw.bits & 0xFF)
		bw.bits = 0
		bw.bitsLen = 0
		bw.bitsPos = 0
	}
	return bw.flushBuffer(true)
}

func (bw *bitsWriter) addBits(bits uint32, bitsLen uint8) {
	if bw.bitsLen > 0 {
		bw.bits |= bits << bw.bitsLen
		bw.bitsLen += bitsLen
	} else {
		bw.bits = bits
		bw.bitsLen = bitsLen
		bw.bitsPos = bw.bufPos
		bw.bufPos++
	}
}

func (bw *bitsWriter) flushBits() {
	bw.buf[bw.bitsPos] = byte(bw.bits)
	bw.bits >>= 8
	bw.bitsLen -= 8
	bw.bitsPos = bw.bufPos
	bw.bufPos++
}

func (bw *bitsWriter) extendBuffer() {
	if bw.bufPos >= bw.bufUpSize {
		bw.buf = append(bw.buf, bw.buf...)
		bw.bufUpSize = len(bw.buf) - 8
	}
}

func (bw *bitsWriter) writeBit(bit uint32) {
	if bit != 0 {
		bit = 1
	}
	bw.addBits(bit, 1)
	if bw.bitsLen > 8 {
		bw.flushBits()
	}
	bw.extendBuffer()
}

func (bw *bitsWriter) writeBits(bits uint32, bitsLen uint8) {
	if bitsLen < 1 || bitsLen > 24 {
		panic("Invalid bitsLen")
	}
	bw.addBits(bits&(1<<bitsLen-1), bitsLen)
	for bw.bitsLen > 8 {
		bw.flushBits()
	}
	bw.extendBuffer()
}

func (bw *bitsWriter) writeByte(b byte) {
	bw.buf[bw.bufPos] = b
	bw.bufPos++
	bw.extendBuffer()
}
