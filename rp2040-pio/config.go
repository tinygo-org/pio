//go:build rp2040
// +build rp2040

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
func (cfg *StateMachineConfig) SetClkDivIntFrac(div uint16, frac uint8) {
	cfg.ClkDiv = (uint32(frac) << rp.PIO0_SM0_CLKDIV_FRAC_Pos) |
		(uint32(div) << rp.PIO0_SM0_CLKDIV_INT_Pos)
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
func (cfg *StateMachineConfig) SetOutShift(shiftRight bool, autoPush bool, pushThreshold uint16) {
	cfg.ShiftCtrl = cfg.ShiftCtrl &
		^uint32(rp.PIO0_SM0_SHIFTCTRL_OUT_SHIFTDIR_Msk|
			rp.PIO0_SM0_SHIFTCTRL_AUTOPULL_Msk|
			rp.PIO0_SM0_SHIFTCTRL_PULL_THRESH_Msk) |
		(boolToBit(shiftRight) << rp.PIO0_SM0_SHIFTCTRL_OUT_SHIFTDIR_Pos) |
		(boolToBit(autoPush) << rp.PIO0_SM0_SHIFTCTRL_AUTOPULL_Pos) |
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
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_SIDESET_BASE_Msk)) |
		(uint32(firstPin) << rp.PIO0_SM0_PINCTRL_SIDESET_BASE_Pos)
}

// SetOutPins sets the pins a PIO 'set' instruction modifies.
//   - Base defines the lowest-numbered pin that will be affected by an OUT PINS,
//     OUT PINDIRS or MOV PINS instruction. The data written to this pin will always be
//     the least-significant bit of the OUT or MOV data.
//   - Count defines the number of pins that will be affected by an OUT PINS, 0..32 inclusive.
func (cfg *StateMachineConfig) SetOutPins(base machine.Pin, count uint8) {
	if count > 32 {
		panic("SetSetPins: count")
	}
	cfg.PinCtrl = (cfg.PinCtrl & ^uint32(rp.PIO0_SM0_PINCTRL_SET_BASE_Msk|rp.PIO0_SM0_PINCTRL_SET_COUNT_Msk)) |
		(uint32(base) << rp.PIO0_SM0_PINCTRL_SET_BASE_Pos) |
		(uint32(count) << rp.PIO0_SM0_PINCTRL_SET_COUNT_Pos)
}

type FifoJoin int

const (
	FIFO_JOIN_NONE FifoJoin = iota
	FIFO_JOIN_TX
	FIFO_JOIN_RX
)

// SetFIFOJoin Setup the FIFO joining in a state machine configuration.
func (cfg *StateMachineConfig) SetFIFOJoin(join FifoJoin) {
	/*
		static inline void sm_config_set_fifo_join(pio_sm_config *c, enum pio_fifo_join join) {
		    valid_params_if(PIO, join == PIO_FIFO_JOIN_NONE || join == PIO_FIFO_JOIN_TX || join == PIO_FIFO_JOIN_RX);
		    c->shiftctrl = (c->shiftctrl & (uint)~(PIO_SM0_SHIFTCTRL_FJOIN_TX_BITS | PIO_SM0_SHIFTCTRL_FJOIN_RX_BITS)) |
		                   (((uint)join) << PIO_SM0_SHIFTCTRL_FJOIN_TX_LSB);
		}
	*/
	if join > FIFO_JOIN_RX {
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
