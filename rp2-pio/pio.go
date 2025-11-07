//go:build rp2040 || rp2350

package pio

import (
	"device/rp"
	"errors"
	"machine"
	"runtime/volatile"
	"unsafe"
)

// RP2040 PIO peripheral handles.
var (
	PIO0 = &PIO{
		hw: rp.PIO0,
	}
	PIO1 = &PIO{
		hw: rp.PIO1,
	}
)

// PIO errors.
var (
	ErrOutOfProgramSpace   = errors.New("pio: out of program space")
	ErrNoSpaceAtOffset     = errors.New("pio: program space unavailable at offset")
	errStateMachineClaimed = errors.New("pio: state machine already claimed")
)

const (
	badStateMachineIndex = "invalid state machine index"
	badPIO               = "invalid PIO"
	badProgramBounds     = "invalid program bounds"
)

// PIO represents one of the two PIO peripherals in the RP2040
type PIO struct {
	// hw points to the PIO hardware registers.
	hw *rp.PIO0_Type
	// Bitmask of used instruction space. Each PIO has 32 slots for instructions.
	usedSpaceMask uint32
	// Bitmask of used state machines. Each PIO has 4 state machines.
	claimedSMMask uint8
	nc            noCopy
}

// BlockIndex returns 0, 1, or 2 depending on whether the underlying device is PIO0, PIO1, or PIO2.
func (pio *PIO) BlockIndex() uint8 {
	return pio.blockIndex()
}

// StateMachine returns a state machine by index.
func (pio *PIO) StateMachine(index uint8) StateMachine {
	if index > 3 {
		panic(badStateMachineIndex)
	}
	return StateMachine{
		pio:   pio,
		index: index,
	}
}

// ClaimtateMachine returns an unused state machine
// or an error if all state machines on this PIO are claimed.
func (pio *PIO) ClaimStateMachine() (sm StateMachine, err error) {
	for i := uint8(0); i < 4; i++ {
		sm = pio.StateMachine(i)
		if sm.TryClaim() {
			return sm, nil
		}
	}
	return StateMachine{}, errStateMachineClaimed
}

// AddProgram loads a PIO program into PIO memory and returns the offset where it was loaded.
// This function will try to find the next available slot of memory for the program
// and will return an error if there is not enough memory to add the program.
//
// The instructions argument holds program binary code in 16-bit words.
// origin indicates where in the PIO execution memory the program must be loaded,
// or -1 if the code is position independent.
func (pio *PIO) AddProgram(instructions []uint16, origin int8) (offset uint8, _ error) {
	maybeOffset := pio.findOffsetForProgram(instructions, origin)
	if maybeOffset < 0 {
		return 0, ErrOutOfProgramSpace
	}
	offset = uint8(maybeOffset)
	pio.AddProgramAtOffset(instructions, origin, offset)
	return offset, nil
}

// AddProgramAtOffset loads a PIO program into PIO memory at a specific offset
// and returns a non-nil error if there is not enough space.
func (pio *PIO) AddProgramAtOffset(instructions []uint16, origin int8, offset uint8) error {
	if !pio.CanAddProgramAtOffset(instructions, origin, offset) {
		return ErrNoSpaceAtOffset
	}

	programLen := uint8(len(instructions))
	for i := uint8(0); i < programLen; i++ {
		instr := instructions[i]

		// Patch jump instructions with relative offset
		if _INSTR_BITS_JMP == instr&_INSTR_BITS_Msk {
			pio.writeInstructionMemory(offset+i, instr+uint16(offset))
		} else {
			pio.writeInstructionMemory(offset+i, instr)
		}
	}

	// Mark the instruction space as in-use
	programMask := uint32((1 << programLen) - 1)
	pio.usedSpaceMask |= programMask << uint32(offset)
	return nil
}

// CanAddProgramAtOffset returns true if there is enough space for program at given offset.
func (pio *PIO) CanAddProgramAtOffset(instructions []uint16, origin int8, offset uint8) bool {
	// Non-relocatable programs must be added at offset
	if origin >= 0 && origin != int8(offset) {
		return false
	}

	programMask := uint32((1 << len(instructions)) - 1)
	return pio.usedSpaceMask&(programMask<<offset) == 0
}

func (pio *PIO) writeInstructionMemory(offset uint8, value uint16) {
	// Instead of using MEM0, MEM1, etc, calculate the offset of the
	// disired register starting at MEM0
	start := unsafe.Pointer(&pio.hw.INSTR_MEM0)

	// Instruction Memory registers are 32-bit, with only lower 16 used
	reg := (*volatile.Register32)(unsafe.Pointer(uintptr(start) + uintptr(offset)*4))
	reg.Set(uint32(value))
}

func (pio *PIO) findOffsetForProgram(instructions []uint16, origin int8) int8 {
	programLen := uint32(len(instructions))
	programMask := uint32((1 << programLen) - 1)

	// Program has fixed offset (not relocatable)
	if origin >= 0 {
		if uint32(origin) > 32-programLen {
			return -1
		}

		if (pio.usedSpaceMask & (programMask << origin)) != 0 {
			return -1
		}

		return origin
	}

	// work down from the top always
	for i := int8(32 - programLen); i >= 0; i-- {
		if pio.usedSpaceMask&(programMask<<uint32(i)) == 0 {
			return i
		}
	}

	return -1
}

// ClearProgramSection clears a contiguous section of the PIO's program memory.
// To clear all program memory use ClearProgramSection(0, 32).
func (pio *PIO) ClearProgramSection(offset, len uint8) {
	if offset+len > 32 { // 32 instructions max
		panic(badProgramBounds)
	}
	hw := pio.HW()
	for i := offset; i < offset+len; i++ {
		// We encode trap instructions to prevent undefined behaviour if
		// a state machine is currently using the program memory.
		hw.INSTR_MEM[i].Set(uint32(AssemblerV0{}.Jmp(offset, JmpAlways).Encode()))
	}
	pio.usedSpaceMask &^= uint32((1<<len)-1) << offset
}

type statemachineHW struct {
	CLKDIV    volatile.Register32 // 0xC8 for SM0
	EXECCTRL  volatile.Register32 // 0xCC for SM0
	SHIFTCTRL volatile.Register32 // 0xD0 for SM0
	ADDR      volatile.Register32 // 0xD4 for SM0
	INSTR     volatile.Register32 // 0xD8 for SM0
	PINCTRL   volatile.Register32 // 0xDC for SM0
}

func (pio *PIO) smHW(index uint8) *statemachineHW {
	if index > 3 {
		panic(badStateMachineIndex)
	}
	// 24 bytes (6 registers) per state machine
	const size = unsafe.Sizeof(statemachineHW{})

	ptrBase := unsafe.Pointer(&pio.hw.SM0_CLKDIV) // 0xC8
	ptr := uintptr(ptrBase) + uintptr(index)*size

	return (*statemachineHW)(unsafe.Pointer(uintptr(ptr)))
}

// PinMode returns the PinMode for a PIO state machine, one of
// PIO0, PIO1, or PIO2.
func (pio *PIO) PinMode() machine.PinMode {
	return machine.PinPIO0 + machine.PinMode(pio.BlockIndex())
}

// GetIRQ gets lowest octet of PIO IRQ register.
// State machine IRQ flags register. There are 8
// state machine IRQ flags, which can be set, cleared, and waited on
// by the state machines. There’s no fixed association between
// flags and state machines — any state machine can use any flag.
// Any of the 8 flags can be used for timing synchronisation
// between state machines, using IRQ and WAIT instructions. The
// lower four of these flags are also routed out to system-level
// interrupt requests, alongside FIFO status interrupts — see e.g.
// IRQ0_INTE.
func (pio *PIO) GetIRQ() uint8 {
	return uint8(pio.hw.GetIRQ())
}

// ClearIRQ clears IRQ flags when 1 is written to bit flag.
func (pio *PIO) ClearIRQ(irqMask uint8) {
	pio.hw.SetIRQ(uint32(irqMask))
}

// SetInputSyncBypassMasked sets the pinMask bits of the INPUT_SYNC_BYPASS register
// with the values in the corresponding bypassMask bits.
//
// There is a 2-flipflop synchronizer on each GPIO input, which protects
// PIO logic from metastabilities. This increases input delay, and for
// fast synchronous IO (e.g. SPI) these synchronizers may need to be bypassed.
// If bit set the corresponding synchronizer is bypassed. If in doubt leave as zeros.
func (pio *PIO) SetInputSyncBypassMasked(bypassMask, pinMask uint32) {
	pio.hw.INPUT_SYNC_BYPASS.ReplaceBits(bypassMask, pinMask, 0)
}

// GPIOStates returns the current PIO-commanded state for output GPIOs.
// This allows for debugging of a PIO program using setting or side-setting
// GPIO pins as signals.
func (pio *PIO) GPIOStates() uint32 {
	return pio.hw.DBG_PADOUT.Get()
}

// GPIODirections returns the current PIO-commanded pin directions (Output Enable).
// Useful for debugging the state of a PIO program that swaps pin directions.
func (pio *PIO) GPIODirections() uint32 {
	return pio.hw.DBG_PADOE.Get()
}

const (
	// This bit address is retroactively declared valid as the PIO hardware version
	// for RP2040 (which is 0) according to the RP2350 datasheet, but undefined in
	// its datasheet or SVD so we define it here for compatibility.
	pio0_SM0_DBG_CFGINFO_VERSION_Pos = 0x1C
)

// Version returns the version of the PIO hardware.
// 0 for RP2040, 1 for RP2350.
func (pio *PIO) Version() uint8 {
	return uint8(pio.hw.DBG_CFGINFO.Get() >> pio0_SM0_DBG_CFGINFO_VERSION_Pos)
}

// HW returns a pointer to the PIO's hardware registers.
func (pio *PIO) HW() *pioHW { return (*pioHW)(unsafe.Pointer(pio.hw)) }

// Programmable IO block
type pioHW struct {
	CTRL              volatile.Register32 // 0x0
	FSTAT             volatile.Register32 // 0x4
	FDEBUG            volatile.Register32 // 0x8
	FLEVEL            volatile.Register32 // 0xC
	TXF               [4]volatile.Register32
	RXF               [4]volatile.Register32
	IRQ               volatile.Register32                       // 0x30
	IRQ_FORCE         volatile.Register32                       // 0x34
	INPUT_SYNC_BYPASS volatile.Register32                       // 0x38
	DBG_PADOUT        volatile.Register32                       // 0x3C
	DBG_PADOE         volatile.Register32                       // 0x40
	DBG_CFGINFO       volatile.Register32                       // 0x44
	INSTR_MEM         [32]volatile.Register32                   // 0x48..0xC4
	SM                [4]statemachineHW                         // SM0=[0xC8..0xDC], .. 0x124
	RXF_PUTGET        [rp2350ExtraReg][4][4]volatile.Register32 // ----- | 0x128
	GPIOBASE          [rp2350ExtraReg]volatile.Register32       // ----- | 0x168
	INTR              volatile.Register32                       // 0x128 | 0x16C
	IRQ_INT           [2]irqINTHW                               // 0x12C..0x140 | 0x170..0x184
}

type irqINTHW struct {
	E volatile.Register32
	F volatile.Register32
	S volatile.Register32
}

const (
	sizeOK = unsafe.Sizeof(rp.PIO0_Type{}) == unsafe.Sizeof(pioHW{})
)

// noCopy may be embedded into structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) UnLock() {}
