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
		HW: rp.PIO0,
	}
	PIO1 = &PIO{
		HW: rp.PIO1,
	}
)

// PIO errors.
var (
	ErrOutOfProgramSpace = errors.New("pio: out of program space")
	ErrNoSpaceAtOffset   = errors.New("pio: program space unavailable at offset")
)

// PIO represents one of the two PIO peripherals in the RP2040
type PIO struct {
	// Bitmask of used instruction space
	usedSpaceMask uint32
	// HW is the actual hardware device
	HW *rp.PIO0_Type
}

// StateMachine represents one of the four state machines in a PIO
type StateMachine struct {
	// The pio containing this state machine
	pio *PIO

	// index of this state machine
	index uint8
}

// StateMachineIndex returns the index of the state machine within the PIO.
func (sm StateMachine) StateMachineIndex() uint8 {
	return sm.index
}

// BlockIndex returns 0 or 1 depending on whether the underlying device is PIO0 or PIO1.
func (pio *PIO) BlockIndex() uint8 {
	switch pio.HW {
	case rp.PIO0:
		return 0
	case rp.PIO1:
		return 1
	}
	panic("invalid PIO")
}

// StateMachine returns a state machine by index.
func (pio *PIO) StateMachine(index uint8) StateMachine {
	if index > 3 {
		panic("invalid state machine index")
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
	pio.AddProgramAtOffset(instructions, origin, uint8(offset))
	return uint8(maybeOffset), nil
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
	start := unsafe.Pointer(&pio.HW.INSTR_MEM0)

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

// Init initializes the state machine
//
// initialPC is the initial program counter
// cfg is optional.  If the zero value of StateMachineConfig is used
// then the default configuration is used.
func (sm StateMachine) Init(initialPC uint8, cfg StateMachineConfig) {
	// Halt the state machine to set sensible defaults
	sm.SetEnabled(false)
	sm.IsTxFIFOEmpty()
	if cfg == (StateMachineConfig{}) {
		cfg = DefaultStateMachineConfig()
		sm.SetConfig(cfg)
	} else {
		sm.SetConfig(cfg)
	}

	sm.ClearFIFOs()

	// Clear FIFO debug flags
	fdebugMask := uint32((1 << rp.PIO0_FDEBUG_TXOVER_Pos) |
		(1 << rp.PIO0_FDEBUG_RXUNDER_Pos) |
		(1 << rp.PIO0_FDEBUG_TXSTALL_Pos) |
		(1 << rp.PIO0_FDEBUG_RXSTALL_Pos))
	sm.pio.HW.FDEBUG.Set(fdebugMask << sm.index)

	sm.Restart()
	sm.ClkDivRestart()
	sm.Exec(EncodeJmp(uint16(initialPC)))
}

// SetEnabled controls whether the state machine is running
func (sm StateMachine) SetEnabled(enabled bool) {
	sm.pio.HW.CTRL.ReplaceBits(boolToBit(enabled), 0x1, sm.index)
}

// Restart restarts the state machine
func (sm StateMachine) Restart() {
	sm.pio.HW.CTRL.SetBits(1 << (rp.PIO0_CTRL_SM_RESTART_Pos + sm.index))
}

// Restart a state machine clock divider with a phase of 0
func (sm StateMachine) ClkDivRestart() {
	sm.pio.HW.CTRL.SetBits(1 << (rp.PIO0_CTRL_CLKDIV_RESTART_Pos + sm.index))
}

// SetConfig applies state machine configuration to a state machine
func (sm StateMachine) SetConfig(cfg StateMachineConfig) {
	hw := sm.HW()
	hw.CLKDIV.Set(cfg.ClkDiv)
	hw.EXECCTRL.Set(cfg.ExecCtrl)
	hw.SHIFTCTRL.Set(cfg.ShiftCtrl)
	hw.PINCTRL.Set(cfg.PinCtrl)
}

// tx gets a pointer to the TX FIFO register for this state machine.
func (sm StateMachine) tx() *volatile.Register32 {
	start := unsafe.Pointer(&sm.pio.HW.TXF0)
	offset := uintptr(sm.index) * 4
	return (*volatile.Register32)(unsafe.Pointer(uintptr(start) + offset))
}

// rx gets a pointer to the RX FIFO register for this state machine.
func (sm StateMachine) rx() *volatile.Register32 {
	start := unsafe.Pointer(&sm.pio.HW.RXF0)
	offset := uintptr(sm.index) * 4
	return (*volatile.Register32)(unsafe.Pointer(uintptr(start) + offset))
}

// SetConsecurityPinDirs sets a range of pins to either 'in' or 'out'.
func (sm StateMachine) SetConsecutivePinDirs(pin machine.Pin, count uint8, isOut bool) {
	pinctl := &sm.HW().PINCTRL

	pinctrl_saved := pinctl.Get()
	pindir_val := uint16(0)
	if isOut {
		pindir_val = 0x1f
	}

	for count > 5 {
		pinctl.Set((5 << rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos) | (uint32(pin) << rp.PIO0_SM0_PINCTRL_SET_BASE_Pos))
		sm.Exec(EncodeSet(SrcDestPinDirs, pindir_val))
		count -= 5
		pin = (pin + 5) & 0x1f
	}

	pinctl.Set((uint32(count) << rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos) | (uint32(pin) << rp.PIO0_SM0_PINCTRL_SET_BASE_Pos))
	sm.Exec(EncodeSet(SrcDestPinDirs, pindir_val))
	pinctl.Set(pinctrl_saved)
}

// TxPut puts a value into the state machine's TX FIFO.
//
// This function does not check for fullness. If the FIFO is full the FIFO
// contents are not affected and the sticky TXOVER flag is set for this FIFO in FDEBUG.
func (sm StateMachine) TxPut(data uint32) {
	reg := sm.tx()
	reg.Set(data)
}

// RxGet reads a word of data from a state machine's RX FIFO.
//
// This function does not check for emptiness. If the FIFO is empty
// the result is undefined and the sticky RXUNDER flag for this FIFO is set in FDEBUG.
func (sm StateMachine) RxGet() uint32 {
	reg := sm.rx()
	return reg.Get()
}

// RxFIFOLevel returns the number of elements currently in a state machine's RX FIFO.
// The number of elements returned is in the range 0..15.
func (sm StateMachine) RxFIFOLevel() uint32 {
	const mask = rp.PIO0_FLEVEL_RX0_Msk >> rp.PIO0_FLEVEL_RX0_Pos
	bitoffs := rp.PIO0_FLEVEL_RX0_Pos + sm.index*(rp.PIO0_FLEVEL_RX1_Pos-rp.PIO0_FLEVEL_RX0_Pos)
	return (sm.pio.HW.FLEVEL.Get() >> uint32(bitoffs)) & mask
}

// TxFIFOLevel returns the number of elements currently in a state machine's TX FIFO.
// The number of elements returned is in the range 0..15.
func (sm StateMachine) TxFIFOLevel() uint32 {
	const mask = rp.PIO0_FLEVEL_TX0_Msk >> rp.PIO0_FLEVEL_TX0_Pos
	bitoffs := rp.PIO0_FLEVEL_TX0_Pos + sm.index*(rp.PIO0_FLEVEL_TX1_Pos-rp.PIO0_FLEVEL_TX0_Pos)
	return (sm.pio.HW.FLEVEL.Get() >> uint32(bitoffs)) & mask
}

// IsTxFIFOEmpty returns true if state machine's TX FIFO is empty.
func (sm StateMachine) IsTxFIFOEmpty() bool {
	return (sm.pio.HW.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_TXEMPTY_Pos + sm.index))) != 0
}

// IsTxFIFOFull returns true if state machine's TX FIFO is full.
func (sm StateMachine) IsTxFIFOFull() bool {
	return (sm.pio.HW.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_TXFULL_Pos + sm.index))) != 0
}

// IsRxFIFOEmpty returns true if state machine's RX FIFO is empty.
func (sm StateMachine) IsRxFIFOEmpty() bool {
	return (sm.pio.HW.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_RXEMPTY_Pos + sm.index))) != 0
}

// IsRxFIFOFull returns true if state machine's RX FIFO is full.
func (sm StateMachine) IsRxFIFOFull() bool {
	return (sm.pio.HW.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_RXFULL_Pos + sm.index))) != 0
}

// ClearFIFOs clears the TX and RX FIFOs of a state machine.
func (sm StateMachine) ClearFIFOs() {
	shiftctl := &sm.HW().SHIFTCTRL
	// FIFOs are flushed when this bit is changed. Xoring twice returns bit to original state.
	xorBits(shiftctl, rp.PIO0_SM0_SHIFTCTRL_FJOIN_RX)
	xorBits(shiftctl, rp.PIO0_SM0_SHIFTCTRL_FJOIN_RX)
}

// Exec will immediately execute an instruction on the state machine
func (sm StateMachine) Exec(instr uint16) {
	sm.HW().INSTR.Set(uint32(instr))
}

type statemachineHW struct {
	CLKDIV    volatile.Register32 // 0xC8 for SM0
	EXECCTRL  volatile.Register32 // 0xCC for SM0
	SHIFTCTRL volatile.Register32 // 0xD0 for SM0
	ADDR      volatile.Register32 // 0xD4 for SM0
	INSTR     volatile.Register32 // 0xD8 for SM0
	PINCTRL   volatile.Register32 // 0xDC for SM0
}

// HW returns a pointer to the configuration hardware registers for this state machine.
func (sm StateMachine) HW() *statemachineHW { return sm.pio.smHW(sm.index) }

// PIO returns the PIO that this state machine is part of.
func (sm StateMachine) PIO() *PIO { return sm.pio }

func (pio *PIO) smHW(index uint8) *statemachineHW {
	if index > 3 {
		panic("invalid state machine index")
	}
	const size = unsafe.Sizeof(statemachineHW{})  // 24 bytes.
	ptrBase := unsafe.Pointer(&pio.HW.SM0_CLKDIV) // 0xC8
	ptr := uintptr(ptrBase) + uintptr(index)*size
	return (*statemachineHW)(unsafe.Pointer(uintptr(ptr)))
}

const (
	_REG_ALIAS_RW_BITS  = 0x0 << 12
	_REG_ALIAS_XOR_BITS = 0x1 << 12
	_REG_ALIAS_SET_BITS = 0x2 << 12
	_REG_ALIAS_CLR_BITS = 0x3 << 12
)

// Gets the 'XOR' alias for a register
//
// Registers have 'ALIAS' registers with special semantics, see
// 2.1.2. Atomic Register Access in the RP2040 Datasheet
//
// Each peripheral register block is allocated 4kB of address space, with registers accessed using one of 4 methods,
// selected by address decode.
//   - Addr + 0x0000 : normal read write access
//   - Addr + 0x1000 : atomic XOR on write
//   - Addr + 0x2000 : atomic bitmask set on write
//   - Addr + 0x3000 : atomic bitmask clear on write
func xorRegister(reg *volatile.Register32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(uintptr(unsafe.Pointer(reg)) | _REG_ALIAS_XOR_BITS))
}

func xorBits(reg *volatile.Register32, bits uint32) {
	xorRegister(reg).Set(bits)
}

func boolToBit(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
