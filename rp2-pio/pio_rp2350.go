//go:build rp2350

package pio

import (
	"device/rp"
)

const (
	rp2350ExtraReg = 1
)

// RP2350 PIO peripheral handles.
var (
	PIO2 = &PIO{
		hw: rp.PIO2,
	}
)

func (pio *PIO) blockIndex() uint8 {
	switch pio.hw {
	case rp.PIO0:
		return 0
	case rp.PIO1:
		return 1
	case rp.PIO2:
		return 2
	}
	panic(badPIO)
}

// SetGPIOBase configures the GPIO base for the PIO block, or which GPIO pin is
// seen as pin 0 inside the PIO. Can only be set to values of 0 or 16 and only
// sensible for use on RP2350B.
func (pio *PIO) SetGPIOBase(base uint32) {
	switch base {
	case 0, 16:
		pio.hw.GPIOBASE.Set(base)
	default:
		panic("pio:invalid gpiobase")
	}
}

// SetNextPIOMask configures the 4-bit mask for state machines in the next PIO block
// that should be affected by ClkDivRestart() and SetEnabled() functions on this PIO
// block's state machines, allowing for cycle-perfect synchronization. RP2350-only.
func (pio *PIO) SetNextPIOMask(mask uint32) {
	pio.hw.CTRL.ReplaceBits(mask, rp.PIO0_CTRL_NEXT_PIO_MASK_Msk, rp.PIO0_CTRL_NEXT_PIO_MASK_Pos)
}

// SetPrevPIOMask configures the 4-bit mask for state machines in the previous PIO
// block that should be affected by ClkDivRestart() and SetEnabled() functions on this
// PIO block's state machines, allowing for cycle-perfect synchronization. RP2350-only.
func (pio *PIO) SetPrevPIOMask(mask uint32) {
	pio.hw.CTRL.ReplaceBits(mask, rp.PIO0_CTRL_PREV_PIO_MASK_Msk, rp.PIO0_CTRL_PREV_PIO_MASK_Pos)
}
