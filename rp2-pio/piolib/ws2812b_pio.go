// Code generated by pioasm; DO NOT EDIT.

//go:build rp2040
package piolib
import (
    pio "github.com/tinygo-org/pio/rp2-pio"
)
// ws2812b_led

const ws2812b_ledWrapTarget = 0
const ws2812b_ledWrap = 20

const ws2812b_ledoffset_entry_point = 0

var ws2812b_ledInstructions = []uint16{
		//     .wrap_target
		0x80a0, //  0: pull   block                      
		0xe037, //  1: set    x, 23                      
		0xe001, //  2: set    pins, 1                    
		0x6041, //  3: out    y, 1                       
		0x0066, //  4: jmp    !y, 6                      
		0x0207, //  5: jmp    7                      [2] 
		0xe300, //  6: set    pins, 0                [3] 
		0xe100, //  7: set    pins, 0                [1] 
		0x0042, //  8: jmp    x--, 2                     
		0xbf42, //  9: nop                           [31]
		0xbf42, // 10: nop                           [31]
		0xbf42, // 11: nop                           [31]
		0xbf42, // 12: nop                           [31]
		0xbf42, // 13: nop                           [31]
		0xbf42, // 14: nop                           [31]
		0xbf42, // 15: nop                           [31]
		0xbf42, // 16: nop                           [31]
		0xbf42, // 17: nop                           [31]
		0xbf42, // 18: nop                           [31]
		0xbf42, // 19: nop                           [31]
		0x0d00, // 20: jmp    0                      [13]
		//     .wrap
}
const ws2812b_ledOrigin = -1
func ws2812b_ledProgramDefaultConfig(offset uint8) pio.StateMachineConfig {
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset+ws2812b_ledWrapTarget, offset+ws2812b_ledWrap)
	return cfg;
}

