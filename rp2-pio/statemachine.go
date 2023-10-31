package pio

import (
	"device/rp"
	"machine"
	"math/bits"
	"runtime/volatile"
	"unsafe"
)

// StateMachine represents one of the four state machines in a PIO
type StateMachine struct {
	// The pio containing this state machine
	pio *PIO

	// index of this state machine
	index uint8
}

// IsClaimed returns true if the state machine is claimed by other code and should not be used.
func (sm StateMachine) IsClaimed() bool { return sm.pio.claimedSMMask&(1<<sm.index) != 0 }

// Unclaim releases the state machine for use by other code.
func (sm StateMachine) Unclaim() { sm.pio.claimedSMMask &^= (1 << sm.index) }

// Claim attempts to claim the state machine for use by the caller and returns
// true if successful, or false if StateMachine already claimed.
func (sm StateMachine) Claim() bool {
	if sm.IsClaimed() {
		return false
	}
	sm.pio.claimedSMMask |= 1 << sm.index
	return true
}

// HW returns a pointer to the configuration hardware registers for this state machine.
func (sm StateMachine) HW() *statemachineHW { return sm.pio.smHW(sm.index) }

// PIO returns the PIO that this state machine is part of.
func (sm StateMachine) PIO() *PIO {
	sm.pio.BlockIndex() // Panic if PIO or state machine not at valid offset.
	return sm.pio
}

// StateMachineIndex returns the index of the state machine within the PIO.
func (sm StateMachine) StateMachineIndex() uint8 { return sm.index }

// IsValid returns true if state machine is a valid instance.
func (sm StateMachine) IsValid() bool {
	return (sm.pio.hw == rp.PIO0 || sm.pio.hw == rp.PIO1) && sm.index <= 3
}

// Init initializes the state machine
//
// initialPC is the initial program counter
// cfg is optional.  If the zero value of StateMachineConfig is used
// then the default configuration is used.
func (sm StateMachine) Init(initialPC uint8, cfg StateMachineConfig) {
	sm.PIO().BlockIndex() // Panic if PIO or state machine not at valid offset.
	if sm.index > 3 {
		panic(badStateMachineIndex)
	}

	// Halt the state machine to set sensible defaults
	sm.SetEnabled(false)

	if cfg == (StateMachineConfig{}) {
		cfg = DefaultStateMachineConfig()
		sm.SetConfig(cfg)
	} else {
		sm.SetConfig(cfg)
	}

	sm.ClearFIFOs()

	// Clear FIFO debug flags
	const fdebugMask = uint32((1 << rp.PIO0_FDEBUG_TXOVER_Pos) |
		(1 << rp.PIO0_FDEBUG_RXUNDER_Pos) |
		(1 << rp.PIO0_FDEBUG_TXSTALL_Pos) |
		(1 << rp.PIO0_FDEBUG_RXSTALL_Pos))

	sm.pio.hw.FDEBUG.Set(fdebugMask << sm.index)

	sm.Restart()
	sm.ClkDivRestart()
	sm.Exec(EncodeJmp(uint16(initialPC)))
}

// SetEnabled controls whether the state machine is running
func (sm StateMachine) SetEnabled(enabled bool) {
	sm.pio.hw.CTRL.ReplaceBits(boolToBit(enabled), 0x1, sm.index)
}

// Restart restarts the state machine
func (sm StateMachine) Restart() {
	sm.pio.hw.CTRL.SetBits(1 << (rp.PIO0_CTRL_SM_RESTART_Pos + sm.index))
}

// Restart a state machine clock divider with a phase of 0
func (sm StateMachine) ClkDivRestart() {
	sm.pio.hw.CTRL.SetBits(1 << (rp.PIO0_CTRL_CLKDIV_RESTART_Pos + sm.index))
}

// SetConfig applies state machine configuration to a state machine
func (sm StateMachine) SetConfig(cfg StateMachineConfig) {
	sm.PIO().BlockIndex() // Panic if PIO or state machine not at valid offset.
	if sm.index > 3 {
		panic(badStateMachineIndex)
	}
	hw := sm.HW()
	hw.CLKDIV.Set(cfg.ClkDiv)
	hw.EXECCTRL.Set(cfg.ExecCtrl)
	hw.SHIFTCTRL.Set(cfg.ShiftCtrl)
	hw.PINCTRL.Set(cfg.PinCtrl)
}

// SetClkDiv sets the clock divider for the state machine from a whole and fractional part where:
//
//	Frequency = clock freq / (CLKDIV_INT + CLKDIV_FRAC / 256)
func (sm StateMachine) SetClkDiv(whole uint16, frac uint8) {
	sm.HW().CLKDIV.Set(clkDiv(whole, frac))
}

// TxPut puts a value into the state machine's TX FIFO.
//
// This function does not check for fullness. If the FIFO is full the FIFO
// contents are not affected and the sticky TXOVER flag is set for this FIFO in FDEBUG.
func (sm StateMachine) TxPut(data uint32) {
	reg := sm.TxReg()
	reg.Set(data)
}

// RxGet reads a word of data from a state machine's RX FIFO.
//
// This function does not check for emptiness. If the FIFO is empty
// the result is undefined and the sticky RXUNDER flag for this FIFO is set in FDEBUG.
func (sm StateMachine) RxGet() uint32 {
	reg := sm.RxReg()
	return reg.Get()
}

// TxReg gets a pointer to the TX FIFO register for this state machine.
func (sm StateMachine) TxReg() *volatile.Register32 {
	start := unsafe.Pointer(&sm.pio.hw.TXF0) // 0x10
	offset := uintptr(sm.index) * 4
	return (*volatile.Register32)(unsafe.Pointer(uintptr(start) + offset))
}

// RxReg gets a pointer to the RX FIFO register for this state machine.
func (sm StateMachine) RxReg() *volatile.Register32 {
	start := unsafe.Pointer(&sm.pio.hw.RXF0) // 0x20
	offset := uintptr(sm.index) * 4
	return (*volatile.Register32)(unsafe.Pointer(uintptr(start) + offset))
}

// RxFIFOLevel returns the number of elements currently in a state machine's RX FIFO.
// The number of elements returned is in the range 0..15.
func (sm StateMachine) RxFIFOLevel() uint32 {
	const mask = rp.PIO0_FLEVEL_RX0_Msk >> rp.PIO0_FLEVEL_RX0_Pos
	bitoffs := rp.PIO0_FLEVEL_RX0_Pos + sm.index*(rp.PIO0_FLEVEL_RX1_Pos-rp.PIO0_FLEVEL_RX0_Pos)
	return (sm.pio.hw.FLEVEL.Get() >> uint32(bitoffs)) & mask
}

// TxFIFOLevel returns the number of elements currently in a state machine's TX FIFO.
// The number of elements returned is in the range 0..15.
func (sm StateMachine) TxFIFOLevel() uint32 {
	const mask = rp.PIO0_FLEVEL_TX0_Msk >> rp.PIO0_FLEVEL_TX0_Pos
	bitoffs := rp.PIO0_FLEVEL_TX0_Pos + sm.index*(rp.PIO0_FLEVEL_TX1_Pos-rp.PIO0_FLEVEL_TX0_Pos)
	return (sm.pio.hw.FLEVEL.Get() >> uint32(bitoffs)) & mask
}

// IsTxFIFOEmpty returns true if state machine's TX FIFO is empty.
func (sm StateMachine) IsTxFIFOEmpty() bool {
	return (sm.pio.hw.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_TXEMPTY_Pos + sm.index))) != 0
}

// IsTxFIFOFull returns true if state machine's TX FIFO is full.
func (sm StateMachine) IsTxFIFOFull() bool {
	return (sm.pio.hw.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_TXFULL_Pos + sm.index))) != 0
}

// IsRxFIFOEmpty returns true if state machine's RX FIFO is empty.
func (sm StateMachine) IsRxFIFOEmpty() bool {
	return (sm.pio.hw.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_RXEMPTY_Pos + sm.index))) != 0
}

// IsRxFIFOFull returns true if state machine's RX FIFO is full.
func (sm StateMachine) IsRxFIFOFull() bool {
	return (sm.pio.hw.FSTAT.Get() & (1 << (rp.PIO0_FSTAT_RXFULL_Pos + sm.index))) != 0
}

// ClearFIFOs clears the TX and RX FIFOs of a state machine.
func (sm StateMachine) ClearFIFOs() {
	hw := sm.HW()
	shiftctl := &hw.SHIFTCTRL
	// FIFOs are flushed when this bit is changed. Xoring twice returns bit to original state.
	xorBits(shiftctl, rp.PIO0_SM0_SHIFTCTRL_FJOIN_RX_Msk)
	xorBits(shiftctl, rp.PIO0_SM0_SHIFTCTRL_FJOIN_RX_Msk)
}

// Exec will immediately execute an instruction on the state machine
func (sm StateMachine) Exec(instr uint16) {
	sm.HW().INSTR.Set(uint32(instr))
}

// SetPindirsConsecutive sets a range of pins to either 'in' or 'out'. This must be done
// for all used pins before the state machine is started, including SET, IN, OUT and SIDESET pins.
func (sm StateMachine) SetPindirsConsecutive(pin machine.Pin, count uint8, isOut bool) {
	checkPinBaseAndCount(pin, count)
	sm.SetPindirsMasked(makePinmask(uint8(pin), count, uint8(boolToBit(isOut))))
}

// SetPinsConsecutive sets a range of pins initial starting values.
func (sm StateMachine) SetPinsConsecutive(pin machine.Pin, count uint8, level bool) {
	checkPinBaseAndCount(pin, count)
	sm.SetPinsMasked(makePinmask(uint8(pin), count, uint8(boolToBit(level))))
}

func makePinmask(base, count, bit uint8) (valMask, pinMask uint32) {
	start := uint8(base)
	end := start + count
	for shift := start; shift < end; shift++ {
		valMask |= uint32(bit) << shift
		pinMask |= 1 << shift
	}
	return valMask, pinMask
}

// SetPinsMasked sets a value on multiple pins for the PIO instance.
// This method repeatedly reconfigures the state machines pins.
// Use this method as convenience to set initial pin states BEFORE running state machine.
func (sm StateMachine) SetPinsMasked(valueMask, pinMask uint32) {
	sm.setPinExec(SrcDestPins, valueMask, pinMask)
}

func (sm StateMachine) SetPindirsMasked(dirMask, pinMask uint32) {
	sm.setPinExec(SrcDestPinDirs, dirMask, pinMask)
}

func (sm StateMachine) setPinExec(dest SrcDest, valueMask, pinMask uint32) {
	hw := sm.HW()
	pinctrlSaved := hw.PINCTRL.Get()
	execctrlSaved := hw.EXECCTRL.Get()
	hw.EXECCTRL.ClearBits(1 << rp.PIO0_SM0_EXECCTRL_OUT_STICKY_Pos)
	// select the algorithm to use. Naive or the pico-sdk way.
	const naive = true
	if naive {
		for i := uint8(0); i < 32; i++ {
			if pinMask&(1<<i) == 0 {
				continue
			}
			hw.PINCTRL.Set(
				1<<rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos |
					uint32(i)<<rp.PIO0_SM0_PINCTRL_SET_BASE_Pos,
			)
			value := 0x1 & uint16(valueMask>>i)
			sm.Exec(EncodeSet(dest, value))
		}
	} else {
		for pinMask != 0 {
			// https://github.com/raspberrypi/pico-sdk/blob/6a7db34ff63345a7badec79ebea3aaef1712f374/src/rp2_common/hardware_pio/pio.c#L178
			base := uint32(bits.TrailingZeros32(pinMask))

			hw.PINCTRL.Set(
				1<<rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos |
					base<<rp.PIO0_SM0_PINCTRL_SET_BASE_Pos,
			)

			value := 0x1 & uint16(valueMask>>base)
			sm.Exec(EncodeSet(dest, value))
			pinMask &= pinMask - 1
		}
	}
	hw.PINCTRL.Set(pinctrlSaved)
	hw.EXECCTRL.Set(execctrlSaved)
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