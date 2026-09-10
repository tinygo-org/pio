//go:build rp2040 || rp2350

package piolib

import "machine"

type parallelPins struct {
	gpioBase uint32
	clock    machine.Pin
	dataBase machine.Pin
	pinMask  uint32
}

// parallelPinMask returns the PIO pin mask for the clock pin and contiguous
// data pins. The pin arguments must already be mapped into the PIO-relative
// GPIOBASE window selected for the state machine.
func parallelPinMask(clock, dataBase machine.Pin, busWidth uint8) uint32 {
	mask := uint32(1) << uint(clock)
	for pinoff := uint8(0); pinoff < busWidth; pinoff++ {
		mask |= uint32(1) << uint(dataBase+machine.Pin(pinoff))
	}
	return mask
}
