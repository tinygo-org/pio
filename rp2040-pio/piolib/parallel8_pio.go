// Code generated by pioasm; DO NOT EDIT.

//go:build rp2040
package piolib
import (
	pio "github.com/tinygo-org/pio/rp2040-pio"
)
// parallel8

const parallel8WrapTarget = 0
const parallel8Wrap = 1

var parallel8Instructions = []uint16{
		//     .wrap_target
		0x6008, //  0: out    pins, 8         side 0     
		0xb142, //  1: nop                    side 1 [1] 
		//     .wrap
}
const parallel8Origin = -1
func parallel8ProgramDefaultConfig(offset uint8) pio.StateMachineConfig {
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset+parallel8WrapTarget, offset+parallel8Wrap)
	cfg.SetSidesetParams(1, false, false)
	return cfg;
}

