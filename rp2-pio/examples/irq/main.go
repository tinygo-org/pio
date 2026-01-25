// Example irq demonstrates PIO interrupt handling.
// A simple PIO program sets IRQ flag 0, and an interrupt handler
// sets a bool to verify the IRQ was triggered.
//go:build rp2040 || rp2350

package main

import (
	"math/bits"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// irqTriggered is set to true when the interrupt handler fires.
// Remember, we can't do anything that blocks from interrupt,
// that includes printing- so we need an external variable.
var irqTriggered bool = false
var irqSource pio.IRQSource = 0xffffffff
var irqLine uint8 = 0xff
var pioBlock uint8 = 0xff

func main() {
	time.Sleep(2 * time.Second)
	println("IRQ example starting...")

	Pio := pio.PIO0

	// Register interrupt handler for IRQ flag 0 on PIO0's interrupt line 0.
	err := Pio.SetInterrupt(0, pio.IRQS0, func(block, irqNum uint8, source pio.IRQSource) {
		irqTriggered = true
		pioBlock = block
		irqLine = irqNum
		irqSource = source
	})
	if err != nil {
		panic("failed to set interrupt: " + err.Error())
	}
	// Simple PIO program that sets IRQ flag 0 once, then loops forever.
	// The program:
	//   irq set 0    ; Set IRQ flag 0
	//   jmp 1        ; Loop forever (jump to self)
	//
	// We must NOT wrap back to the IRQ instruction, otherwise it creates
	// an interrupt storm that overwhelms the CPU.
	asm := pio.AssemblerV0{}
	var (
		irqSetOrigin       int8 = -1
		irqSetInstructions      = []uint16{
			asm.IRQSet(false, 0).Encode(),      // 0: irq set 0
			asm.Jmp(pio.JmpAlways, 1).Encode(), // 1: jmp 1 (loop forever)
		}
	)
	offset, err := Pio.AddProgram(irqSetInstructions, irqSetOrigin)
	if err != nil {
		panic("failed to add program: " + err.Error())
	}
	println("Loaded program at offset", offset)

	sm := Pio.StateMachine(0)
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset+1, offset+1) // Wrap on the jmp instruction (no-op since it jumps to itself)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	// Wait a short time for the IRQ to fire.
	time.Sleep(10 * time.Millisecond)

	// Validate that the interrupt was triggered.
	if irqTriggered {
		println("SUCCESS: IRQ was triggered!")
		println("pioBlock:", pioBlock, "irq:ine:", irqLine, "source:", irqSource, "sourceFirstBitIdx:", bits.TrailingZeros32(uint32(irqSource)))
	} else {
		println("FAILURE: IRQ was NOT triggered")
	}

	// Clean up.
	sm.SetEnabled(false)
	Pio.SetInterrupt(0, pio.IRQS0, nil)
}
