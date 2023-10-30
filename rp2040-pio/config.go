//go:build rp2040

package pio

import (
	"device/rp"
	"machine"
)

// DefaultStateMachineConfig returns the default configuration
// for a PIO state machine.
//
// The default configuration here, mirrors the state from
// pio_get_default_sm_config in the c-sdk.
//
// Note: Function used by pico-sdk's pioasm tool so signature MUST remain the same.
func DefaultStateMachineConfig() StateMachineConfig {
	cfg := StateMachineConfig{}
	cfg.SetClkDivIntFrac(1, 0)
	cfg.SetWrap(0, 31)
	cfg.SetInShift(true, false, 32)
	cfg.SetOutShift(true, false, 32)
	return cfg
}

// StateMachineConfig holds the configuration for a PIO state
// machine.
//
// Note: Type used by pico-sdk's pioasm tool so signature MUST remain the same.
type StateMachineConfig struct {
	// Clock divisor register for state machine N
	//  Frequency = clock freq / (CLKDIV_INT + CLKDIV_FRAC / 256)
	ClkDiv uint32
	// Execution/behavioural settings for state machine N
	ExecCtrl uint32
	// Control behaviour of the input/output shift registers for state machine N.
	ShiftCtrl uint32
	// State machine pin control.
	PinCtrl uint32
}

// SetClkDivIntFrac sets the clock divider for the state
// machine from a whole and fractional part.
//
//	Frequency = clock freq / (CLKDIV_INT + CLKDIV_FRAC / 256)
func (cfg *StateMachineConfig) SetClkDivIntFrac(whole uint16, frac uint8) {
	cfg.ClkDiv = clkDiv(whole, frac)
}

func clkDiv(whole uint16, frac uint8) uint32 {
	return (uint32(frac) << rp.PIO0_SM0_CLKDIV_FRAC_Pos) |
		(uint32(whole) << rp.PIO0_SM0_CLKDIV_INT_Pos)
}

// SetWrap sets the wrapping configuration for the state machine
//
// Note: Function used by pico-sdk's pioasm tool so signature MUST remain the same.
func (cfg *StateMachineConfig) SetWrap(wrapTarget uint8, wrap uint8) {
	cfg.ExecCtrl =
		(cfg.ExecCtrl & ^uint32(rp.PIO0_SM0_EXECCTRL_WRAP_TOP_Msk|rp.PIO0_SM0_EXECCTRL_WRAP_BOTTOM_Msk)) |
			(uint32(wrapTarget) << rp.PIO0_SM0_EXECCTRL_WRAP_BOTTOM_Pos) |
			(uint32(wrap) << rp.PIO0_SM0_EXECCTRL_WRAP_TOP_Pos)
}

// SetInShift sets the 'in' shifting parameters in a state machine configuration
//   - shiftRight is true if ISR shift direction is right, false if left.
//   - autoPush enables automatic ISR refilling after all of the ISR bits have been consumed.
//   - pushThreshold is threshold in bits to shift in before auto/conditional re-pushing of the ISR.
func (cfg *StateMachineConfig) SetInShift(shiftRight bool, autoPush bool, pushThreshold uint16) {
	cfg.ShiftCtrl = cfg.ShiftCtrl &
		^uint32(rp.PIO0_SM0_SHIFTCTRL_IN_SHIFTDIR_Msk|
			rp.PIO0_SM0_SHIFTCTRL_AUTOPUSH_Msk|
			rp.PIO0_SM0_SHIFTCTRL_PUSH_THRESH_Msk) |
		(boolToBit(shiftRight) << rp.PIO0_SM0_SHIFTCTRL_IN_SHIFTDIR_Pos) |
		(boolToBit(autoPush) << rp.PIO0_SM0_SHIFTCTRL_AUTOPUSH_Pos) |
		(uint32(pushThreshold&0x1f) << rp.PIO0_SM0_SHIFTCTRL_PUSH_THRESH_Pos)
}

// SetOutShift sets the 'out' shifting parameters in a state machine configuration
//   - shiftRight is true if OSR shift direction is right, false if left.
//   - autoPull enables automatic OSR refilling after all of the OSR bits have been consumed.
//   - pushThreshold is threshold in bits to shift out before auto/conditional re-pulling of the OSR.
func (cfg *StateMachineConfig) SetOutShift(shiftRight bool, autoPull bool, pushThreshold uint16) {
	cfg.ShiftCtrl = cfg.ShiftCtrl &
		^uint32(rp.PIO0_SM0_SHIFTCTRL_OUT_SHIFTDIR_Msk|
			rp.PIO0_SM0_SHIFTCTRL_AUTOPULL_Msk|
			rp.PIO0_SM0_SHIFTCTRL_PULL_THRESH_Msk) |
		(boolToBit(shiftRight) << rp.PIO0_SM0_SHIFTCTRL_OUT_SHIFTDIR_Pos) |
		(boolToBit(autoPull) << rp.PIO0_SM0_SHIFTCTRL_AUTOPULL_Pos) |
		(uint32(pushThreshold&0x1f) << rp.PIO0_SM0_SHIFTCTRL_PULL_THRESH_Pos)
}

// SetSidesetParams sets the side-set parameters in a state machine configuration.
//   - bitcount is number of bits to steal from delay field in the instruction for use of side set (max 5).
//   - optional is true if the topmost side set bit is used as a flag for whether to apply side set on that instruction.
//   - pindirs is true if the side-set affects pin directions rather than values.
//
// Note: Function used by pico-sdk's pioasm tool so signature MUST remain the same.
func (cfg *StateMachineConfig) SetSidesetParams(bitCount uint8, optional bool, pindirs bool) {
	if bitCount > 5 {
		panic("SetSideSet: bitCount")
	}
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_SIDESET_COUNT_Msk)) |
		(uint32(bitCount) << uint32(rp.PIO0_SM0_PINCTRL_SIDESET_COUNT_Pos))

	cfg.ExecCtrl = (cfg.ExecCtrl & ^uint32(rp.PIO0_SM0_EXECCTRL_SIDE_EN_Msk|rp.PIO0_SM0_EXECCTRL_SIDE_PINDIR_Msk)) |
		(boolToBit(optional) << rp.PIO0_SM0_EXECCTRL_SIDE_EN_Pos) |
		(boolToBit(pindirs) << rp.PIO0_SM0_EXECCTRL_SIDE_PINDIR_Pos)
}

// SetSidesetPins sets the lowest-numbered pin that will be affected by a side-set
// operation.
func (cfg *StateMachineConfig) SetSidesetPins(firstPin machine.Pin) {
	checkPinBaseAndCount(firstPin, 1)
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_SIDESET_BASE_Msk)) |
		(uint32(firstPin) << rp.PIO0_SM0_PINCTRL_SIDESET_BASE_Pos)
}

// SetOutPins sets the pins a PIO 'out' instruction modifies. Can overlap with pins in IN, SET and SIDESET.
// `out` instructions receive data from the OSR (output shift register) and write it to the GPIO pins in bitwise format,
// thus OUT pins are best suited for driving data protocols with multiple data wires.
//   - Base defines the lowest-numbered pin that will be affected by an OUT PINS,
//     OUT PINDIRS or MOV PINS instruction. The data written to this pin will always be
//     the least-significant bit of the OUT or MOV data.
//   - Count defines the number of pins that will be affected by an OUT PINS, 0..32 inclusive.
func (cfg *StateMachineConfig) SetOutPins(base machine.Pin, count uint8) {
	checkPinBaseAndCount(base, count)
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_OUT_BASE_Msk|rp.PIO0_SM0_PINCTRL_OUT_COUNT_Msk)) |
		(uint32(base) << rp.PIO0_SM0_PINCTRL_OUT_BASE_Pos) |
		(uint32(count) << rp.PIO0_SM0_PINCTRL_OUT_COUNT_Pos)
}

// SetSetPins sets the pins a PIO 'set' instruction modifies.
// Can overlap with pins in IN, OUT and SIDESET.
// Set pins are best suited to assert control signals such as clock/chip-selects.
//
// The mapping of SET and OUT onto pins is configured independently. They may be mapped to distinct locations, for example
// if one pin is to be used as a clock signal, and another for data. They may also be overlapping ranges of pins: a UART
// transmitter might use SET to assert start and stop bits, and OUT instructions to shift out FIFO data to the same pins.
func (cfg *StateMachineConfig) SetSetPins(base machine.Pin, count uint8) {
	checkPinBaseAndCount(base, count)
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_SET_BASE_Msk|rp.PIO0_SM0_PINCTRL_SET_COUNT_Msk)) |
		(uint32(base) << rp.PIO0_SM0_PINCTRL_SET_BASE_Pos) |
		(uint32(count) << rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos)
}

// SetInPins in a state machine configuration. Can overlap with OUT, SET and SIDESET pins.
func (cfg *StateMachineConfig) SetInPins(base machine.Pin) {
	checkPinBaseAndCount(base, 1)
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_IN_BASE_Msk)) | (uint32(base) << rp.PIO0_SM0_PINCTRL_IN_BASE_Pos)
}

// SetJmpPin sets the gpio pin to use as the source for a `jmp pin` instruction.
func (cfg *StateMachineConfig) SetJmpPin(pin machine.Pin) {
	checkPinBaseAndCount(pin, 1)
	cfg.ExecCtrl = (cfg.ExecCtrl & ^uint32(rp.PIO0_SM0_EXECCTRL_JMP_PIN_Msk)) | (uint32(pin) << rp.PIO0_SM0_EXECCTRL_JMP_PIN_Pos)
}

// SetOutSpecial set special 'out' operations in a state machine configuration.
//   - sticky to enable 'sticky' output (i.e. re-asserting most recent OUT/SET pin values on subsequent cycles).
//   - hasEnablePin true to enable auxiliary OUT enable pin.
//   - enable pin for auxiliary OUT enable.
func (cfg *StateMachineConfig) SetOutSpecial(sticky, hasEnablePin bool, enable machine.Pin) {
	cfg.ExecCtrl = (cfg.ExecCtrl &
		^uint32(rp.PIO0_SM0_EXECCTRL_OUT_STICKY_Msk|rp.PIO0_SM0_EXECCTRL_INLINE_OUT_EN_Msk|
			rp.PIO0_SM0_EXECCTRL_OUT_EN_SEL_Msk)) |
		(boolToBit(sticky) << rp.PIO0_SM0_EXECCTRL_OUT_STICKY_Pos) |
		(boolToBit(hasEnablePin) << rp.PIO0_SM0_EXECCTRL_INLINE_OUT_EN_Pos) |
		((uint32(enable) << rp.PIO0_SM0_EXECCTRL_OUT_EN_SEL_Pos) & rp.PIO0_SM0_EXECCTRL_OUT_EN_SEL_Msk)
}

// SetMovStatus sets source for 'mov status' in a state machine configuration.
//   - statusSel is the status operation selector.
//   - statusN parameter for the mov status operation (currently a bit count).
func (cfg *StateMachineConfig) SetMovStatus(statusSel MovStatus, statusN uint32) {
	cfg.ExecCtrl = (cfg.ExecCtrl &
		^uint32(rp.PIO0_SM0_EXECCTRL_STATUS_SEL_Msk|rp.PIO0_SM0_EXECCTRL_STATUS_N_Msk)) |
		((uint32(statusSel) << rp.PIO0_SM0_EXECCTRL_STATUS_SEL_Pos) & rp.PIO0_SM0_EXECCTRL_STATUS_SEL_Msk) |
		((statusN << rp.PIO0_SM0_EXECCTRL_STATUS_N_Pos) & rp.PIO0_SM0_EXECCTRL_STATUS_N_Msk)
}

func checkPinBaseAndCount(base machine.Pin, count uint8) {
	if base >= 32 {
		panic("pio:bad pin")
	} else if count > 32 {
		panic("pio:count too large")
	}
}

type FifoJoin uint8

const (
	// FifoJoinNone is the default FIFO joining configuration. The RX and TX FIFOs are separate and of length 4 each.
	FifoJoinNone FifoJoin = iota
	// FifoJoinTx joins the RX and TX FIFOs into a single TX FIFO of depth 8.
	FifoJoinTx
	// FifoJoinRx joins the RX and TX FIFOs into a single RX FIFO of depth 8.
	FifoJoinRx
)

// MOV status types.
type MovStatus uint8

const (
	MovStatusTxLessthan MovStatus = iota
	MovStatusRxLessthan
)

// SetFIFOJoin Setup the FIFO joining in a state machine configuration.
func (cfg *StateMachineConfig) SetFIFOJoin(join FifoJoin) {
	if join > FifoJoinRx {
		panic("SetFIFOJoin: join")
	}
	cfg.ShiftCtrl = (cfg.ShiftCtrl & ^uint32(rp.PIO0_SM0_SHIFTCTRL_FJOIN_TX_Msk|rp.PIO0_SM0_SHIFTCTRL_FJOIN_RX_Msk)) |
		(uint32(join) << rp.PIO0_SM0_SHIFTCTRL_FJOIN_TX_Pos)
}

func boolToBit(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
