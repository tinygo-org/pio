package piolib

import (
	"errors"
	"runtime"
)

var (
	errTimeout = errors.New("piolib:timeout")
	errBusy    = errors.New("piolib:busy")
)

//go:generate pioasm -o go parallel8.pio  parallel8_pio.go
//go:generate pioasm -o go pulsar.pio     pulsar_pio.go
//go:generate pioasm -o go spi.pio        spi_pio.go
//go:generate pioasm -o go ws2812.pio     ws2812_pio.go
//go:generate pioasm -o go i2s.pio        i2s_pio.go
//go:generate pioasm -o go sdio.pio       sdio_pio.go

func gosched() {
	runtime.Gosched()
}
