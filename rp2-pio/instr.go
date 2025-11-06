package pio

import (
	"errors"
	"math"
)

// 5 bits of delay/sideset.
const delaySidesetbits = 0b1_1111 << 8

// Assembler provides a fluent API for programming PIO
// within the Go language. Here's an example Parallel API
// with a variable bus width:
//
//	// Translation of below program:
//	// 0: out    pins, <npins>   side 0
//	// 1: nop                    side 1
//	// 2: nop                    side 0
//	var program = [3]uint16{
//		asm.Out(pio.SrcDestPins, cfg.BusWidth).Side(0).Encode(),
//		asm.Nop().Side(1).Encode(),
//		asm.Nop().Side(0).Encode(),
//	}
type Assembler struct {
	SidesetBits uint8
}

type instruction struct {
	instr uint16
	asm   Assembler
}

func (instr instruction) Encode() uint16 {
	return instr.instr
}

func (instr instruction) Side(value uint8) instruction {
	instr.instr &^= instr.asm.sidesetbits()
	instr.instr |= EncodeSideSet(instr.asm.SidesetBits, value) // TODO: panic on bit overflow.
	return instr
}

func (instr instruction) Delay(cycles uint8) instruction {
	instr.instr &^= instr.asm.delaybits()
	instr.instr |= EncodeDelay(cycles) // TODO: panic on bit overflow due to sideset bits excess.
	return instr
}

func (asm Assembler) sidesetbits() uint16 {
	return delaySidesetbits & (uint16(0b111) << (13 - asm.SidesetBits))
}

func (asm Assembler) delaybits() uint16 {
	return delaySidesetbits & (0b11111 << (8 - asm.SidesetBits))
}

func (asm Assembler) instr(instr uint16) instruction {
	return instruction{instr: instr, asm: asm}
}

func (asm Assembler) Out(dest SrcDest, value uint8) instruction {
	return asm.instr(EncodeOut(dest, value))
}

func (asm Assembler) Nop() instruction {
	return asm.instr(EncodeNOP())
}

func (asm Assembler) Jmp(addr uint8, cond JmpCond) instruction {
	return asm.instr(EncodeJmp(addr, cond))
}

func (asm Assembler) In(src SrcDest, value uint8) instruction {
	return asm.instr(EncodeIn(src, value))
}

func (asm Assembler) Pull(ifEmpty, block bool) instruction {
	return asm.instr(EncodePull(ifEmpty, block))
}

func (asm Assembler) Push(ifFull, block bool) instruction {
	return asm.instr(EncodePush(ifFull, block))
}

func (asm Assembler) Mov(dest, src SrcDest) instruction {
	return asm.instr(EncodeMov(dest, src))
}

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
	return uint16(0b11111&cycles) << 8
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

// encodeTRAP encodes a trap instruction. It must be stored at the argument offset.
func encodeTRAP(trapOffset uint8) uint16 {
	return EncodeJmp(trapOffset, JmpAlways)
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
