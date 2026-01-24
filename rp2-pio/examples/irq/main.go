// Example irq demonstrates PIO interrupt handling.
// A simple PIO program sets IRQ flag 0, and an interrupt handler
// sets a bool to verify the IRQ was triggered.
//go:build rp2040 || rp2350

package main

import (
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// irqTriggered is set to true when the interrupt handler fires.
var irqTriggered bool

func main() {
	time.Sleep(2 * time.Second)
	println("IRQ example starting...")

	Pio := pio.PIO0

	// Register interrupt handler for IRQ flag 0 on PIO0's interrupt line 0.
	err := Pio.SetInterrupt(0, pio.IRQS0, func(block, irqNum uint8, source pio.IRQSource) {
		irqTriggered = true
	})
	if err != nil {
		panic("failed to set interrupt: " + err.Error())
	}
	// Simple PIO program that immediately sets IRQ flag 0.
	// The program:
	//   irq set 0    ; Set IRQ flag 0
	//   .wrap        ; Loop forever (back to irq set)
	//
	// We use wrap to loop back so the program continues running,
	// but the IRQ should trigger on the first instruction.
	var (
		irqSetOrigin       int8 = -1
		irqSetInstructions      = []uint16{
			pio.AssemblerV0{}.IRQSet(false, 0).Encode(), // irq set 0
		}
	)
	offset, err := Pio.AddProgram(irqSetInstructions, irqSetOrigin)
	if err != nil {
		panic("failed to add program: " + err.Error())
	}
	println("Loaded program at offset", offset)

	sm := Pio.StateMachine(0)
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset, offset) // Wrap from instruction 0 back to 0
	println("init")
	sm.Init(offset, cfg)
	println("enable")
	sm.SetEnabled(true)
	println("sleep")
	// Wait a short time for the IRQ to fire.
	time.Sleep(10 * time.Millisecond)

	// Validate that the interrupt was triggered.
	if irqTriggered {
		println("SUCCESS: IRQ was triggered!")
	} else {
		println("FAILURE: IRQ was NOT triggered")
	}

	// Clean up.
	sm.SetEnabled(false)
	Pio.SetInterrupt(0, pio.IRQS0, nil)
}
