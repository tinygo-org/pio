//go:build rp2040
// +build rp2040

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
	ErrOutOfProgramSpace = errors.New("pio: out of program space")
	ErrNoSpaceAtOffset   = errors.New("pio: program space unavailable at offset")
)

const badStateMachineIndex = "invalid state machine index"
const badPIO = "invalid PIO"

// PIO represents one of the two PIO peripherals in the RP2040
type PIO struct {
	// Bitmask of used instruction space
	usedSpaceMask uint32
	// HW is the actual hardware device
	hw *rp.PIO0_Type
}

// HW returns a pointer to the PIO's hardware registers.
func (pio *PIO) HW() *rp.PIO0_Type { return pio.hw }

// BlockIndex returns 0 or 1 depending on whether the underlying device is PIO0 or PIO1.
func (pio *PIO) BlockIndex() uint8 {
	switch pio.hw {
	case rp.PIO0:
		return 0
	case rp.PIO1:
		return 1
	}
	panic(badPIO)
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
		if INSTR_BITS_JMP == instr&INSTR_BITS_Msk {
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

// PinMode returns the PinMode for a PIO state machine, either PIO0 or PIO1.
func (pio *PIO) PinMode() machine.PinMode {
	return machine.PinPIO0 + machine.PinMode(pio.BlockIndex())
}
