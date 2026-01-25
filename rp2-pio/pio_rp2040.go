//go:build rp2040

package pio

import (
	"device/rp"
	"machine"
	"runtime/interrupt"
)

const (
	rp2350ExtraReg = 0
	numPIO         = 2

	// validINTEBits defines valid interrupt source bits for RP2040.
	// RP2040 only supports 12 bits: FIFO status (bits 0-7) and IRQ flags 0-3 (bits 8-11).
	// IRQ flags 4-7 exist in the IRQ register but cannot trigger CPU interrupts.
	validINTEBits IRQSource = 0x0FFF
)

func getPIO(block uint8) (pio *PIO) {
	switch block {
	case 0:
		return PIO0
	case 1:
		return PIO1
	}
	panic("invalid block")
}

func (pio *PIO) blockIndex() uint8 {
	switch pio.hw {
	case rp.PIO0:
		return 0
	case rp.PIO1:
		return 1
	}
	panic(badPIO)
}

const _NUMIRQ = 32

// Enable or disable a specific interrupt on the executing core.
// num is the interrupt number which must be in [0,31].
func irqSet(num uint32, enabled bool) {
	if num >= _NUMIRQ {
		return
	}
	irqSetMask(1<<num, enabled)
}

func irqSetMask(mask uint32, enabled bool) {
	if false {
		(machine.Pin).SetInterrupt(0, 0, nil) // See tinygo implementation.
	}
	if enabled {
		// Clear pending before enable
		// (if IRQ is actually asserted, it will immediately re-pend)
		rp.PPB.NVIC_ICPR.Set(mask)
		rp.PPB.NVIC_ISER.Set(mask)
	} else {
		rp.PPB.NVIC_ICER.Set(mask)
	}
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
	}
}
