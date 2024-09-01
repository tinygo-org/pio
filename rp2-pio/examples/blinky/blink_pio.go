// Code generated by pioasm; DO NOT EDIT.

package main
import (
    "machine"
    pio "github.com/tinygo-org/pio/rp2-pio"
)
// this is a raw helper function for use by the user which sets up the GPIO output, and configures the SM to output on a particular pin
func blinkProgramInit(sm pio.StateMachine, offset uint8, pin machine.Pin) {
	pin.Configure(machine.PinConfig{Mode: sm.PIO().PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)
	cfg := blinkProgramDefaultConfig(offset)
	cfg.SetSetPins(pin, 1)
	sm.Init(offset, cfg)
}
// blink

const blinkWrapTarget = 2
const blinkWrap = 7

var blinkInstructions = []uint16{
		0x80a0, //  0: pull   block                      
		0x6040, //  1: out    y, 32                      
		//     .wrap_target
		0xa022, //  2: mov    x, y                       
		0xe001, //  3: set    pins, 1                    
		0x0044, //  4: jmp    x--, 4                     
		0xa022, //  5: mov    x, y                       
		0xe000, //  6: set    pins, 0                    
		0x0047, //  7: jmp    x--, 7                     
		//     .wrap
}
const blinkOrigin = -1
func blinkProgramDefaultConfig(offset uint8) pio.StateMachineConfig {
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset+blinkWrapTarget, offset+blinkWrap)
	return cfg;
}

