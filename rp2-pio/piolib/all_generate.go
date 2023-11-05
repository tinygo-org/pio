package piolib

import (
	"errors"
	"runtime"
)

const timeoutRetries = 1023

var (
	errTimeout = errors.New("piolib:timeout")
	errBusy    = errors.New("piolib:busy")

	errDMAUnavail = errors.New("piolib:DMA channel unavailable")
)

//go:generate pioasm -o go parallel8.pio  parallel8_pio.go
//go:generate pioasm -o go pulsar.pio     pulsar_pio.go
//go:generate pioasm -o go spi.pio        spi_pio.go
//go:generate pioasm -o go ws2812.pio     ws2812_pio.go
//go:generate pioasm -o go i2s.pio        i2s_pio.go
//go:generate pioasm -o go spi3w.pio       spi3w_pio.go
func gosched() {
	runtime.Gosched()
}
