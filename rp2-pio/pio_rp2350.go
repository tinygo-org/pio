//go:build rp2350

package pio

import (
	"device/rp"
	"machine"
	"runtime/interrupt"
)

const (
	rp2350ExtraReg = 1
	numPIO         = 3
	_NUMIRQ        = 52

	// validINTEBits defines valid interrupt source bits for RP2350.
	// RP2350 supports all 16 bits: FIFO status (bits 0-7) and all IRQ flags 0-7 (bits 8-15).
	validINTEBits IRQSource = 0xFFFF
)

// RP2350 PIO peripheral handles.
var (
	PIO2 = &PIO{
		hw: rp.PIO2,
	}
)

func getPIO(block uint8) (pio *PIO) {
	switch block {
	case 0:
		return PIO0
	case 1:
		return PIO1
	case 2:
		return PIO2
	}
	panic("invalid block")
}

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

func interruptSet(nblock, irq uint8) {
	// Need big switch since interrupt.New needs go constant for interrupt ID.
	switch {
	case nblock == 0 && irq == 0:
		interrupt.New(rp.IRQ_PIO0_IRQ_0, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO0_IRQ_0, true)
	case nblock == 0 && irq == 1:
		interrupt.New(rp.IRQ_PIO0_IRQ_1, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO0_IRQ_1, true)
	case nblock == 1 && irq == 0:
		interrupt.New(rp.IRQ_PIO1_IRQ_0, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO1_IRQ_0, true)
	case nblock == 1 && irq == 1:
		interrupt.New(rp.IRQ_PIO1_IRQ_1, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO1_IRQ_1, true)
	case nblock == 2 && irq == 0:
		interrupt.New(rp.IRQ_PIO2_IRQ_0, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO2_IRQ_0, true)
	case nblock == 2 && irq == 1:
		interrupt.New(rp.IRQ_PIO2_IRQ_1, handleInterrupt).Enable()
		irqSet(rp.IRQ_PIO2_IRQ_1, true)
	}
}

// Enable or disable a specific interrupt on the executing core.
// num is the interrupt number which must be in [0,31].
func irqSet(num uint32, enabled bool) {
	if num >= _NUMIRQ {
		return
	}
	irqSetMask(num/32, 1<<num, enabled)
}

func irqSetMask(n uint32, mask uint32, enabled bool) {
	if false {
		(machine.Pin).SetInterrupt(0, 0, nil) // See tinygo implementation.
	}
	icpr := &rp.PPB.NVIC_ICPR0
	iser := &rp.PPB.NVIC_ISER0
	icer := &rp.PPB.NVIC_ICER0
	if n > 0 {
		icpr = &rp.PPB.NVIC_ICPR1
		iser = &rp.PPB.NVIC_ISER1
		icer = &rp.PPB.NVIC_ICER1
	}
	if enabled {
		icpr.Set(mask)
		iser.Set(mask)
	} else {
		icer.Set(mask)
	}
}
