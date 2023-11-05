package pio

import (
	"errors"
	"math"
)

// InstrKind is a enum for the PIO instruction type. It only represents the kind of
// instruction. It cannot store the arguments.
type InstrKind uint8

const (
	InstrJMP InstrKind = iota
	InstrWAIT
	InstrIN
	InstrOUT
	InstrPUSH
	InstrPULL
	InstrMOV
	InstrIRQ
	InstrSET
)

// This file contains the primitives for creating instructions dynamically
const (
	_INSTR_BITS_JMP  = 0x0000
	_INSTR_BITS_WAIT = 0x2000
	_INSTR_BITS_IN   = 0x4000
	_INSTR_BITS_OUT  = 0x6000
	_INSTR_BITS_PUSH = 0x8000
	_INSTR_BITS_PULL = 0x8080
	_INSTR_BITS_MOV  = 0xa000
	_INSTR_BITS_IRQ  = 0xc000
	_INSTR_BITS_SET  = 0xe000

	// Bit mask for instruction code
	_INSTR_BITS_Msk = 0xe000
)

type SrcDest uint8

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

type JmpCond uint8

const (
	// No condition, always jumps.
	JmpAlways JmpCond = iota
	// Jump if X is zero.
	JmpXZero
	// Jump if X is not zero, prior to decrement of X.
	JmpXNZeroDec
	// Jump if Y is zero.
	JmpYZero
	// Jump if Y is not zero, prior to decrement of Y.
	JmpYNZeroDec
	// Jump if X is not equal to Y.
	JmpXNotEqualY
	// Jump if EXECCTRL_JMP_PIN (state machine configured) is high.
	JmpPinInput
	// Compares the bits shifted out since last pull with the shift count theshold
	// (configured by SHIFTCTRL_PULL_THRESH) and jumps if there are remaining bits to shift.
	JmpOSRNotEmpty
)

// EncodeInstr encodes an arbitrary PIO instruction with the given arguments.
func EncodeInstr(instr InstrKind, delaySideset, arg1_3b, arg2_5b uint8) uint16 {
	return uint16(instr&0b111)<<13 | uint16(delaySideset&0x1f)<<8 | uint16(arg1_3b&0b111)<<5 | uint16(arg2_5b&0x1f)
}

func majorInstrBits(instr uint16) uint16 {
	return instr & _INSTR_BITS_Msk
}

func encodeInstrAndArgs(instr uint16, arg1 uint8, arg2 uint8) uint16 {
	return instr | (uint16(arg1) << 5) | uint16(arg2&0x1f)
}

func encodeInstrAndSrcDest(instr uint16, dest SrcDest, value uint8) uint16 {
	return encodeInstrAndArgs(instr, uint8(dest)&7, value)
}

func EncodeDelay(cycles uint8) uint16 {
	return 0b11111 & (uint16(cycles) << 8)
}

func EncodeSideSet(bitCount, value uint8) uint16 {
	return uint16(value) << (13 - bitCount)
}

func EncodeSetSetOpt(bitCount uint8, value uint8) uint16 {
	return 0x1000 | uint16(value)<<(12-bitCount)
}

func EncodeJmp(addr uint8, condition JmpCond) uint16 {
	return encodeInstrAndArgs(_INSTR_BITS_JMP, uint8(condition&0b111), addr)
}

func encodeIRQ(relative bool, irq uint8) uint8 {
	return boolAsU8(relative) << 4
}

func EncodeWaitGPIO(polarity bool, pin uint8) uint16 {
	flag := boolAsU8(polarity) << 2
	return encodeInstrAndArgs(_INSTR_BITS_WAIT, 0|flag, pin)
}

func EncodeWaitPin(polarity bool, pin uint8) uint16 {
	flag := boolAsU8(polarity) << 2

	return encodeInstrAndArgs(_INSTR_BITS_WAIT, 1|flag, pin)
}

func EncodeWaitIRQ(polarity bool, relative bool, irq uint8) uint16 {
	flag := boolAsU8(polarity) << 2

	return encodeInstrAndArgs(_INSTR_BITS_WAIT, 2|flag, encodeIRQ(relative, irq))
}

func EncodeIn(src SrcDest, value uint8) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_IN, src, value)
}

func EncodeOut(dest SrcDest, value uint8) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_OUT, dest, value)
}

func EncodePush(ifFull bool, block bool) uint16 {
	arg := boolAsU8(ifFull)<<1 | boolAsU8(block)
	return encodeInstrAndArgs(_INSTR_BITS_PUSH, arg, 0)
}

func EncodePull(ifEmpty bool, block bool) uint16 {
	arg := boolAsU8(ifEmpty)<<1 | boolAsU8(block)
	return encodeInstrAndArgs(_INSTR_BITS_PULL, arg, 0)
}

func EncodeMov(dest SrcDest, src SrcDest) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_MOV, dest, uint8(src)&7)
}

func EncodeMovNot(dest SrcDest, src SrcDest) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_MOV, dest, (1<<3)|(uint8(src)&7))
}

func EncodeMovReverse(dest SrcDest, src SrcDest) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_MOV, dest, (2<<3)|(uint8(src)&7))
}

func EncodeIRQSet(relative bool, irq uint8) uint16 {
	return encodeInstrAndArgs(_INSTR_BITS_IRQ, 0, encodeIRQ(relative, irq))
}

func EncodeIRQClear(relative bool, irq uint8) uint16 {
	return encodeInstrAndArgs(_INSTR_BITS_IRQ, 2, encodeIRQ(relative, irq))
}

func EncodeSet(dest SrcDest, value uint8) uint16 {
	return encodeInstrAndSrcDest(_INSTR_BITS_SET, dest, value)
}

func EncodeNOP() uint16 {
	return EncodeMov(SrcDestY, SrcDestY)
}

// ClkDivFromPeriod calculates the CLKDIV register values
// to reach a given StateMachine cycle period given the RP2040 CPU frequency.
// period is expected to be in nanoseconds. freq is expected to be in Hz.
//
// Prefer using ClkDivFromFrequency if possible for speed and accuracy.
func ClkDivFromPeriod(period, cpuFreq uint32) (whole uint16, frac uint8, err error) {
	//  freq = 256*clockfreq / (256*whole + frac)
	// where period = 1e9/freq => freq = 1e9/period, so:
	//  1e9/period = 256*clockfreq / (256*whole + frac) =>
	//  256*whole + frac = 256*clockfreq*period/1e9
	return splitClkdiv(256 * uint64(period) * uint64(cpuFreq) / uint64(1e9))
}

// ClkDivFromFrequency calculates the CLKDIV register values
// to reach a given StateMachine cycle frequency. freq and cpuFreq are expected to be in Hz.
//
// Use powers of two for freq to avoid slow divisions and rounding errors.
func ClkDivFromFrequency(freq, cpuFreq uint32) (whole uint16, frac uint8, err error) {
	//  freq = 256*clockfreq / (256*whole + frac)
	//  256*whole + frac = 256*clockfreq / freq
	return splitClkdiv(256 * uint64(cpuFreq) / uint64(freq))

}

func splitClkdiv(clkdiv uint64) (whole uint16, frac uint8, err error) {
	if clkdiv > 256*math.MaxUint16 {
		return 0, 0, errors.New("ClkDiv: too large period or CPU frequency")
	} else if clkdiv < 256 {
		return 0, 0, errors.New("ClkDiv: too small period or CPU frequency")
	}
	whole = uint16(clkdiv / 256)
	frac = uint8(clkdiv % 256)
	return whole, frac, nil
}

func boolAsU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
