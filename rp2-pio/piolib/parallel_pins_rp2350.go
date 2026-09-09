//go:build rp2350

package piolib

import (
	"errors"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// parallelPinConfig validates RP2350 parallel bus pins and maps physical GPIO
// numbers into the PIO-relative GPIOBASE window needed to address them.
func parallelPinConfig(cfg ParallelConfig) (parallelPins, error) {
	clock := uint32(cfg.Clock)
	dataBase := uint32(cfg.DataBase)
	dataEnd := dataBase + uint32(cfg.BusWidth) - 1

	if clock > 47 || dataEnd > 47 {
		return parallelPins{}, errors.New("parallel pins exceed supported GPIO range")
	}

	if clock < 32 && dataEnd < 32 {
		return parallelPins{
			clock:    cfg.Clock,
			dataBase: cfg.DataBase,
			pinMask:  parallelPinMask(cfg.Clock, cfg.DataBase, cfg.BusWidth),
		}, nil
	}

	if clock < 16 || dataBase < 16 {
		return parallelPins{}, errors.New("parallel pins span incompatible GPIOBASE windows")
	}

	relativeClock := cfg.Clock - 16
	relativeDataBase := cfg.DataBase - 16
	return parallelPins{
		gpioBase: 16,
		clock:    relativeClock,
		dataBase: relativeDataBase,
		pinMask:  parallelPinMask(relativeClock, relativeDataBase, cfg.BusWidth),
	}, nil
}

// setParallelGPIOBase applies the selected PIO GPIOBASE window on RP2350.
func setParallelGPIOBase(pio *pio.PIO, base uint32) {
	pio.SetGPIOBase(base)
}
