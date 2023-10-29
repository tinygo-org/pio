package piolib

import (
	"errors"
	"runtime"
)

var errTimeout = errors.New("piolib:timeout")

//go:generate pioasm -o go parallel8.pio  parallel8_pio.go
//go:generate pioasm -o go pulsar.pio     pulsar_pio.go
//go:generate pioasm -o go spi.pio        spi_pio.go
//go:generate pioasm -o go ws2812.pio     ws2812_pio.go

func gosched() {
	runtime.Gosched()
}
