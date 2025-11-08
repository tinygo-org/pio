package pio

import (
	"errors"
	"math"
)

// 5 bits of delay/sideset.
const delaySidesetbits = 0b1_1111 << 8

// AssemblerV0 provides a fluent API for programming PIO
// within the Go language for PIO version 0 (RP2040).
//
// Here's an example Parallel API
// with a variable bus width:
//
//	// Translation of below program:
//	// 0: out    pins, <npins>   side 0
//	// 1: nop                    side 1
//	// 2: nop                    side 0
//	var program = [3]uint16{
//		asm.Out(pio.SrcDestPins, numberOfPins).Side(0).Encode(),
//		asm.Nop().Side(1).Encode(),
//		asm.Nop().Side(0).Encode(),
//	}
type AssemblerV0 struct {
	SidesetBits uint8
}

type instructionV0 struct {
	instr uint16
	asm   AssemblerV0
}

// EncodeInstr encodes an arbitrary PIO instruction with the given arguments.
func (asm AssemblerV0) EncodeInstr(instr InstrKind, delaySideset, arg1_3b, arg2_5b uint8) uint16 {
	return uint16(instr&0b111)<<13 | uint16(delaySideset&0x1f)<<8 | uint16(arg1_3b&0b111)<<5 | uint16(arg2_5b&0x1f)
}

func (asm AssemblerV0) encodeIRQ(relative bool, irq uint8) uint8 {
	return boolAsU8(relative)<<4 | irq&0b111
}

func (instr instructionV0) majorbits() uint16 {
	return instr.instr & _INSTR_BITS_Msk
}

func (asm AssemblerV0) instrArgs(instr uint16, arg1_5b uint8, arg2 uint8) instructionV0 {
	return asm.instr(instr | (uint16(arg1_5b) << 5) | uint16(arg2&0x1f))
}

func (asm AssemblerV0) instrSrcDest(instr uint16, srcDest uint8, value uint8) instructionV0 {
	return asm.instrArgs(instr, srcDest&7, value)
}

// Encode returns the finalized assembled instruction ready to be stored to the PIO program memory and used by a PIO state machine.
func (instr instructionV0) Encode() uint16 {
	return instr.instr
}

// Side sets the sideset functionality of an instruction.
//
// value (see Section 3.3.2) is applied to the side_set pins at the start of the instruction. Note that
// the rules for a side-set value via side <side_set_value> are dependent on the .side_set (see
// pioasm_side_set) directive for the program. If no .side_set is specified then the side <side_set_value>
// is invalid, if an optional number of sideset pins is specified then side <side_set_value> may be
// present, and if a non-optional number of sideset pins is specified, then side <side_set_value> is
// required. The <side_set_value> must fit within the number of side-set bits specified in the .side_set
// directive.
func (instr instructionV0) Side(value uint8) instructionV0 {
	instr.instr &^= instr.asm.sidesetbits()
	instr.instr |= uint16(value) << (13 - instr.asm.SidesetBits) // TODO: panic on bit overflow.
	return instr
}

// Delay sets the delay functionality of an instruction.
//
// cycles specifies amount of cycles to delay after the instruction completes. The delay_value is
// specified as a value (see Section 3.3.2), and in general is between 0 and 31 inclusive (a 5-bit
// value), however the number of bits is reduced when sideset is enabled via the .side_set (see
// pioasm_side_set) directive. If the <delay_value> is not present, then the instruction has no delay
func (instr instructionV0) Delay(cycles uint8) instructionV0 {
	instr.instr &^= instr.asm.delaybits()
	instr.instr |= uint16(0b11111&cycles) << 8 // TODO: panic on bit overflow due to sideset bits excess.
	return instr
}

func (asm AssemblerV0) sidesetbits() uint16 {
	return delaySidesetbits & (uint16(0b111) << (13 - asm.SidesetBits))
}

func (asm AssemblerV0) delaybits() uint16 {
	return delaySidesetbits & (0b11111 << (8 - asm.SidesetBits))
}

func (asm AssemblerV0) instr(instr uint16) instructionV0 {
	return instructionV0{instr: instr, asm: asm}
}

// Set program counter to Address if Condition is true, otherwise no operation.
// Delay cycles on a JMP always take effect, whether Condition is true or false, and they take place after Condition is
// evaluated and the program counter is updated.
func (asm AssemblerV0) Jmp(addr uint8, cond JmpCond) instructionV0 {
	return asm.instrArgs(_INSTR_BITS_JMP, uint8(cond&0b111), addr)
}

// WaitPin stalls until Input pin selected by Index. This state machine’s input IO mapping is applied first, and then Index
// selects which of the mapped bits to wait on. In other words, the pin is selected by adding Index to the
// PINCTRL_IN_BASE configuration, modulo 32.
func (asm AssemblerV0) WaitPin(polarity bool, pin uint8) instructionV0 {
	flag := boolAsU8(polarity) << 2
	return asm.instrArgs(_INSTR_BITS_WAIT, 1|flag, pin)
}

// WaitIRQ stalls until PIO IRQ flag selected by irqindex. This IRQ behaves differently to other WAIT sources.
//   - If Polarity is 1, the selected IRQ flag is cleared by the state machine upon the wait condition being met.
//   - The flag index is decoded in the same way as the IRQ index field: if the MSB is set, the state machine ID (0…3) is
//     added to the IRQ index, by way of modulo-4 addition on the two LSBs. For example, state machine 2 with a flag
//     value of '0x11' will wait on flag 3, and a flag value of '0x13' will wait on flag 1. This allows multiple state machines
//     running the same program to synchronise with each other.
func (asm AssemblerV0) WaitIRQ(polarity, relative bool, irqindex uint8) instructionV0 {
	flag := boolAsU8(polarity) << 2
	return asm.instrArgs(_INSTR_BITS_WAIT, 2|flag, asm.encodeIRQ(relative, irqindex))
}

// WaitGPIO stalls until System GPIO input selected by Index. This is an absolute GPIO index, and is not affected by the state machine’s input IO mapping.
func (asm AssemblerV0) WaitGPIO(polarity bool, pin uint8) instructionV0 {
	flag := boolAsU8(polarity) << 2
	return asm.instrArgs(_INSTR_BITS_WAIT, 0|flag, pin)
}

// Shift Bit count bits from Source into the Input Shift Register (ISR). Shift direction is configured for each state machine by
// SHIFTCTRL_IN_SHIFTDIR. Additionally, increase the input shift count by Bit count, saturating at 32.
func (asm AssemblerV0) In(src InSrc, value uint8) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_IN, uint8(src), value)
}

// Shift Bit count bits out of the Output Shift Register (OSR), and write those bits to Destination. Additionally, increase the
// output shift count by Bit count, saturating at 32.
func (asm AssemblerV0) Out(dest OutDest, value uint8) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_OUT, uint8(dest), value)
}

// Push the contents of the ISR into the RX FIFO, as a single 32-bit word. Clear ISR to all-zeroes.
//   - IfFull: If 1, do nothing unless the total input shift count has reached its threshold, SHIFTCTRL_PUSH_THRESH (the same
//     as for autopush; see Section 3.5.4).
//   - Block: If 1, stall execution if RX FIFO is full.
func (asm AssemblerV0) Push(ifFull, block bool) instructionV0 {
	arg := boolAsU8(ifFull)<<1 | boolAsU8(block)
	return asm.instrArgs(_INSTR_BITS_PUSH, arg, 0)
}

// Load a 32-bit word from the TX FIFO into the OSR.
//   - ifEmpty: If 1, do nothing unless the total output shift count has reached its threshold, SHIFTCTRL_PULL_THRESH (the
//     same as for autopull; see Section 3.5.4).
//   - Block: If 1, stall if TX FIFO is empty. If 0, pulling from an empty FIFO copies scratch X to OSR.
func (asm AssemblerV0) Pull(ifEmpty, block bool) instructionV0 {
	arg := boolAsU8(ifEmpty)<<1 | boolAsU8(block)
	return asm.instrArgs(_INSTR_BITS_PULL, arg, 0)
}

// Mov copies data from src to dest.
func (asm AssemblerV0) Mov(dest MovDest, src MovSrc) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_MOV, uint8(dest), uint8(src)&7)
}

// MovInvertBits does a Mov but inverting the resulting bits.
func (asm AssemblerV0) MovInvert(dest MovDest, src MovSrc) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_MOV, uint8(dest), (1<<3)|uint8(src&7))
}

// MovReverse does a Mov but reversing the order of the resulting bits.
func (asm AssemblerV0) MovReverse(dest MovDest, src MovSrc) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_MOV, uint8(dest), (2<<3)|uint8(src&7))
}

// IRQSet sets the IRQ flag selected by irqIndex argument.
func (asm AssemblerV0) IRQSet(relative bool, irqIndex uint8) instructionV0 {
	return asm.instrArgs(_INSTR_BITS_IRQ, 0, asm.encodeIRQ(relative, irqIndex))
}

// IRQClear clears the IRQ flag selected by irqIndex argument. See [AssemblerV0.IRQSet].
func (asm AssemblerV0) IRQClear(relative bool, irqIndex uint8) instructionV0 {
	return asm.instrArgs(_INSTR_BITS_IRQ, 2, asm.encodeIRQ(relative, irqIndex))
}

// Set writes an immediate value Data in range 0..31 to Destination.
func (asm AssemblerV0) Set(dest SetDest, value uint8) instructionV0 {
	return asm.instrSrcDest(_INSTR_BITS_SET, uint8(dest), value)
}

// Nop is pseudo instruction that lasts a single PIO cycle. Usually used for timings.
func (asm AssemblerV0) Nop() instructionV0 { return asm.Mov(MovDestY, MovSrcY) }

// InstrKind is a enum for the PIO instruction type. It only represents the kind of
// instruction. It cannot store the arguments.
type InstrKind uint8

const (
	InstrJMP  InstrKind = iota // jmp
	InstrWAIT                  // wait
	InstrIN                    // in
	InstrOUT                   // out
	InstrPUSH                  // push
	InstrPULL                  // pull
	InstrMOV                   // mov
	InstrIRQ                   // irq
	InstrSET                   // set
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

// OutDest encodes Out instruction data destination.
type OutDest uint8

const (
	OutDestPins    OutDest = 0b000 // pins
	OutDestX       OutDest = 0b001 // x
	OutDestY       OutDest = 0b010 // y
	OutDestNull    OutDest = 0b011 // null
	OutDestPindirs OutDest = 0b100 // pindirs
	OutDestPC      OutDest = 0b101 // pc
	OutDestISR     OutDest = 0b110 // isr
	OutDestExec    OutDest = 0b111 // exec
)

// InSrc encodes In instruction data source.
type InSrc uint8

const (
	InSrcPins InSrc = 0b000 // pins
	InSrcX    InSrc = 0b001 // x
	InSrcY    InSrc = 0b010 // y
	InSrcNull InSrc = 0b011 // null
	InSrcISR  InSrc = 0b110 // isr
	InSrcOSR  InSrc = 0b111 // osr
)

// SetDest encodes Set instruction data destination.
type SetDest uint8

const (
	SetDestPins    SetDest = 0b000 // pins
	SetDestX       SetDest = 0b001 // x
	SetDestY       SetDest = 0b010 // y
	SetDestPindirs SetDest = 0b100 // pindirs
)

// MovSrc encodes Mov instruction data source.
type MovSrc uint8

const (
	MovSrcPins   MovSrc = 0b000 // pins
	MovSrcX      MovSrc = 0b001 // x
	MovSrcY      MovSrc = 0b010 // y
	MovSrcNull   MovSrc = 0b011 // null
	MovSrcStatus MovSrc = 0b101 // status
	MovSrcISR    MovSrc = 0b110 // isr
	MovSrcOSR    MovSrc = 0b111 // osr
)

// MovDest encodes Mov instruction data destination.
type MovDest uint8

const (
	MovDestPins MovDest = 0b000 // pins
	MovDestX    MovDest = 0b001 // x
	MovDestY    MovDest = 0b010 // y
	// MovDestPindirs was introduced in PIO version 1. Not available on RP2040
	MovDestPindirs MovDest = 0b011 // pindirs
	MovDestExec    MovDest = 0b100 // exec
	MovDestPC      MovDest = 0b101 // pc
	MovDestISR     MovDest = 0b110 // isr
	MovDestOSR     MovDest = 0b111 // osr
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

// IRQIndexMode modifies the behaviour if the Index field, either modifying the index, or indexing IRQ flags from a different PIO block.
type IRQIndexMode uint8

const (
	// Prev: the instruction references an IRQ flag from the next-lower-numbered PIO in the system, wrapping to
	// the highest-numbered PIO if this is PIO0. Available on RP2350 only.
	IRQPrev IRQIndexMode = 0b01
	// Rel: the state machine ID (0…3) is added to the IRQ flag index, by way of modulo-4 addition on the two
	// LSBs. For example, state machine 2 with a flag value of '0x11' will wait on flag 3, and a flag value of '0x13' will
	// wait on flag 1. This allows multiple state machines running the same program to synchronise with each other.
	IRQRel IRQIndexMode = 0b10
	// Next: the instruction references an IRQ flag from the next-higher-numbered PIO in the system, wrapping to
	// PIO0 if this is the highest-numbered PIO. Available on RP2350 only.
	IRQNext IRQIndexMode = 0b11
)

// EncodeInstr encodes an arbitrary PIO instruction with the given arguments.
func EncodeInstr(instr InstrKind, delaySideset, arg1_3b, arg2_5b uint8) uint16 {
	return uint16(instr&0b111)<<13 | uint16(delaySideset&0x1f)<<8 | uint16(arg1_3b&0b111)<<5 | uint16(arg2_5b&0x1f)
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
