//go:build rp2040

package piolib

import (
	"errors"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// parallelPinConfig validates RP2040 parallel bus pins and returns their
// unchanged PIO-relative mapping. RP2040 PIO can only address GPIO0..31.
func parallelPinConfig(cfg ParallelConfig) (parallelPins, error) {
	if cfg.Clock >= 32 {
		return parallelPins{}, errors.New("parallel pins exceed supported GPIO range")
	}
	if uint32(cfg.DataBase)+uint32(cfg.BusWidth) > 32 {
		return parallelPins{}, errors.New("parallel pins exceed supported GPIO range")
	}

	return parallelPins{
		clock:    cfg.Clock,
		dataBase: cfg.DataBase,
		pinMask:  parallelPinMask(cfg.Clock, cfg.DataBase, cfg.BusWidth),
	}, nil
}

// setParallelGPIOBase is a no-op on RP2040, which does not support PIO
// GPIOBASE remapping.
func setParallelGPIOBase(_ *pio.PIO, _ uint32) {
}
