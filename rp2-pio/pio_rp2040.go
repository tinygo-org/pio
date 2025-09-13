//go:build rp2040

package pio

import (
	"device/rp"
)

const (
	rp2350ExtraReg = 0
)

func (pio *PIO) blockIndex() uint8 {
	switch pio.hw {
	case rp.PIO0:
		return 0
	case rp.PIO1:
		return 1
	}
	panic(badPIO)
}
