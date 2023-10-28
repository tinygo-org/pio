//go:generate pioasm -o go blink.pio blink_pio.go

package main

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

func main() {
	// Sleep to catch prints.
	time.Sleep(2 * time.Second)
	Pio := pio.PIO0

	offset, err := Pio.AddProgram(blinkInstructions, blinkOrigin)
	if err != nil {
		panic(err.Error())
	}
	println("Loaded program at", offset)

	blinkPinForever(Pio.StateMachine(0), offset, machine.LED, 3)
	blinkPinForever(Pio.StateMachine(1), offset, machine.GPIO6, 4)
	blinkPinForever(Pio.StateMachine(2), offset, machine.GPIO11, 1)
}

func blinkPinForever(sm pio.StateMachine, offset uint8, pin machine.Pin, freq uint) {
	blinkProgramInit(sm, offset, pin)
	const clockFreq = 125000000
	sm.SetEnabled(true)
	println("Blinking", int(pin), "at", freq, "Hz")
	sm.TxPut(uint32(clockFreq / (2 * freq)))
}
