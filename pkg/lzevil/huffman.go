package lzevil

import (
	"math"
	"sort"
)

type symbol struct {
	base         uint32
	bits         uint16
	bitsLen      uint8
	extraBitsLen uint8
}

func (sym *symbol) max() uint32 {
	return sym.base + ((1 << sym.extraBitsLen) - 1)
}

func (sym *symbol) contains(val uint32) bool {
	return sym.base <= val && val <= sym.max()
}

type huffmanCoder struct {
	symbols     []symbol
	minBitsLen  uint8
	maxBitsLen  uint8
	minValue    uint32
	maxValue    uint32
	decodeTable []uint16
}

func newHuffmanCoder(symbols []symbol) *huffmanCoder {
	if len(symbols) < 2 {
		panic("Invalid symbols: must be 2 or more symbols")
	}

	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].base < symbols[j].base
	})

	// Check symbols are valid
	var minBitsLen, maxBitsLen uint8 = symbols[0].bitsLen, symbols[0].bitsLen
	for i, sym := range symbols {
		if sym.base >= math.MaxUint16 || sym.extraBitsLen > 24 ||
			sym.bitsLen < 1 || sym.bitsLen > 24 ||
			i > 0 && sym.base != symbols[i-1].max()+1 {
			panic("Invalid symbols: inconsistent")
		}
		for j, sym2 := range symbols {
			if i != j && sym2.bitsLen >= sym.bitsLen &&
				sym2.bits&(1<<sym.bitsLen-1) == sym.bits {
				panic("Invalid symbols: bits collision")
			}
		}
		if sym.bitsLen > maxBitsLen {
			maxBitsLen = sym.bitsLen
		}
		if sym.bitsLen < minBitsLen {
			minBitsLen = sym.bitsLen
		}
	}

	coder := new(huffmanCoder)
	coder.symbols = symbols
	coder.minBitsLen = minBitsLen
	coder.maxBitsLen = maxBitsLen
	coder.minValue = symbols[0].base
	coder.maxValue = symbols[len(symbols)-1].max()
	coder.decodeTable = make([]uint16, 1<<(maxBitsLen+1)-2)
	for _, sym := range coder.symbols {
		index := int((1 << sym.bitsLen) + sym.bits - 2)
		if sym.extraBitsLen >= 1<<4 || sym.base+1 >= 1<<12 || coder.decodeTable[index] != 0 {
			panic("Invalid symbols")
		}

		packedSym := (uint32(sym.extraBitsLen) << 12) | sym.base
		coder.decodeTable[index] = uint16(packedSym + 1)
	}

	return coder
}

func (coder *huffmanCoder) encodeValue(val uint32, bw *bitsWriter) {
	if val < coder.minValue || val > coder.maxValue {
		panic("Invalid value")
	}

	var sym symbol
	for i := range coder.symbols {
		if coder.symbols[i].contains(val) {
			sym = coder.symbols[i]
			break
		}
	}

	bw.writeBits(uint32(sym.bits), sym.bitsLen)
	if sym.extraBitsLen > 0 {
		bw.writeBits(uint32(val-sym.base), sym.extraBitsLen)
	}
}

func (coder *huffmanCoder) decodeValue(br *bitsReader) uint32 {
	var bits uint32
	for bitsLen := uint8(1); bitsLen <= coder.maxBitsLen; bitsLen++ {
		bits |= br.readBit() << (bitsLen - 1)
		index := int((1 << bitsLen) + bits - 2)
		packedSym := coder.decodeTable[index]
		if packedSym == 0 {
			continue
		}
		packedSym--

		base, extraBitsLen := uint32(packedSym&(1<<12-1)), uint8(packedSym>>12)
		if extraBitsLen > 0 {
			return base + br.readBits(extraBitsLen)
		}
		return base
	}
	panic("Invalid code")
}
