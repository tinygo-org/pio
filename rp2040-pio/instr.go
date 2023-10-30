package pio

import (
	"errors"
	"math"
)

// This file contains the primitives for creating instructions dynamically
const (
	INSTR_BITS_JMP  = 0x0000
	INSTR_BITS_WAIT = 0x2000
	INSTR_BITS_IN   = 0x4000
	INSTR_BITS_OUT  = 0x6000
	INSTR_BITS_PUSH = 0x8000
	INSTR_BITS_PULL = 0x8080
	INSTR_BITS_MOV  = 0xa000
	INSTR_BITS_IRQ  = 0xc000
	INSTR_BITS_SET  = 0xe000

	// Bit mask for instruction code
	INSTR_BITS_Msk = 0xe000
)

type SrcDest uint16

const (
	SrcDestPins    SrcDest = 0
	SrcDestX       SrcDest = 1
	SrcDestY       SrcDest = 2
	SrcDestNull    SrcDest = 3
	SrcDestPinDirs SrcDest = 4
	SrcDestExecMov SrcDest = 4
	SrcDestStatus  SrcDest = 5
	SrcDestPC      SrcDest = 5
	SrcDestISR     SrcDest = 6
	SrcDestOSR     SrcDest = 7
	SrcExecOut     SrcDest = 7
)

func MajorInstrBits(instr uint16) uint16 {
	return instr & INSTR_BITS_Msk
}

func EncodeInstrAndArgs(instr uint16, arg1 uint16, arg2 uint16) uint16 {
	return instr | (arg1 << 5) | (arg2 & 0x1f)
}

func EncodeInstrAndSrcDest(instr uint16, dest SrcDest, value uint16) uint16 {
	return EncodeInstrAndArgs(instr, uint16(dest)&7, value)
}

func EncodeDelay(cycles uint16) uint16 {
	return cycles << 8
}

func EncodeSideSet(bitCount uint16, value uint16) uint16 {
	return value << (13 - bitCount)
}

func EncodeSetSetOpt(bitCount uint16, value uint16) uint16 {
	return 0x1000 | value<<(12-bitCount)
}

func EncodeJmp(addr uint16) uint16 {
	return EncodeInstrAndArgs(INSTR_BITS_JMP, 0, addr)
}

func EncodeIRQ(relative bool, irq uint16) uint16 {
	instr := irq

	if relative {
		instr |= 0x10
	}

	return instr
}

func EncodeWaitGPIO(polarity bool, pin uint16) uint16 {
	flag := uint16(0)
	if polarity {
		flag = 0x4
	}

	return EncodeInstrAndArgs(INSTR_BITS_WAIT, 0|flag, pin)
}

func EncodeWaitPin(polarity bool, pin uint16) uint16 {
	flag := uint16(0)
	if polarity {
		flag = 0x4
	}

	return EncodeInstrAndArgs(INSTR_BITS_WAIT, 1|flag, pin)
}

func EncodeWaitIRQ(polarity bool, relative bool, irq uint16) uint16 {
	flag := uint16(0)
	if polarity {
		flag = 0x4
	}

	return EncodeInstrAndArgs(INSTR_BITS_WAIT, 2|flag, EncodeIRQ(relative, irq))
}

func EncodeIn(src SrcDest, value uint16) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_IN, src, value)
}

func EncodeOut(dest SrcDest, value uint16) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_OUT, dest, value)
}

func EncodePush(ifFull bool, block bool) uint16 {
	arg := uint16(0)
	if ifFull {
		arg |= 2
	}
	if block {
		arg |= 1
	}

	return EncodeInstrAndArgs(INSTR_BITS_PUSH, arg, 0)
}

func EncodePull(ifEmpty bool, block bool) uint16 {
	arg := uint16(0)
	if ifEmpty {
		arg |= 2
	}
	if block {
		arg |= 1
	}

	return EncodeInstrAndArgs(INSTR_BITS_PULL, arg, 0)
}

func EncodeMov(dest SrcDest, src SrcDest) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_MOV, dest, uint16(src)&7)
}

func EncodeMovNot(dest SrcDest, src SrcDest) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_MOV, dest, (1<<3)|(uint16(src)&7))
}

func EncodeMovReverse(dest SrcDest, src SrcDest) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_MOV, dest, (2<<3)|(uint16(src)&7))
}

func EncodeIRQSet(relative bool, irq uint16) uint16 {
	return EncodeInstrAndArgs(INSTR_BITS_IRQ, 0, EncodeIRQ(relative, irq))
}

func EncodeIRQClear(relative bool, irq uint16) uint16 {
	return EncodeInstrAndArgs(INSTR_BITS_IRQ, 2, EncodeIRQ(relative, irq))
}

func EncodeSet(dest SrcDest, value uint16) uint16 {
	return EncodeInstrAndSrcDest(INSTR_BITS_SET, dest, value)
}

func EncodeNOP() uint16 {
	return EncodeMov(SrcDestY, SrcDestY)
}

// ClkDivFromPeriod calculates the CLKDIV register values
// to reach a given StateMachine cycle period given the RP2040 CPU frequency.
// period is expected to be in nanoseconds. freq is expected to be in Hz.
func ClkDivFromPeriod(period, freq uint32) (whole uint16, frac uint8, err error) {
	//  freq = 256*clockfreq / (256*whole + frac)
	// where period = 1e9/freq => freq = 1e9/period, so:
	//  1e9/period = 256*clockfreq / (256*whole + frac) =>
	//  256*whole + frac = 256*clockfreq*period/1e9
	clkdiv := 256 * int64(period) * int64(freq) / int64(1e9)
	if clkdiv > 256*math.MaxUint16 {
		return 0, 0, errors.New("ClkDivFromPeriod: too large period or CPU frequency")
	} else if clkdiv < 256 {
		return 0, 0, errors.New("ClkDivFromPeriod: too small period or CPU frequency")
	}
	whole = uint16(clkdiv / 256)
	frac = uint8(clkdiv % 256)
	return whole, frac, nil
}
