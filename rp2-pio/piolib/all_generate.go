package piolib

import (
	"errors"
	"math"
	"runtime"
	"time"
)

const timeoutRetries = math.MaxUint16 * 8

var (
	errTimeout           = errors.New("piolib:timeout")
	errContentionTimeout = errors.New("piolib:contention timeout")
	errBusy              = errors.New("piolib:busy")

	errDMAUnavail = errors.New("piolib:DMA channel unavailable")
)

//go:generate pioasm -o go parallel6.pio         parallel6_pio.go
//go:generate pioasm -o go parallel8.pio         parallel8_pio.go
//go:generate pioasm -o go pulsar.pio            pulsar_pio.go
//go:generate pioasm -o go spi.pio               spi_pio.go
//go:generate pioasm -o go ws2812b.pio           ws2812b_pio.go
//go:generate pioasm -o go i2s.pio               i2s_pio.go
//go:generate pioasm -o go spi3w.pio             spi3w_pio.go
//go:generate pioasm -o go ws2812bfourpixels.pio ws2812bfourpixels_pio.go

func gosched() {
	runtime.Gosched()
}

type deadline struct {
	t time.Time
}

func (dl deadline) expired() bool {
	if dl.t.IsZero() {
		return false
	}
	return time.Since(dl.t) > 0
}

type deadliner struct {
	// timeout is a bitshift value for the timeout.
	timeout uint8
}

func (ch deadliner) newDeadline() deadline {
	var t time.Time
	if ch.timeout != 0 {
		calc := time.Duration(1 << ch.timeout)
		t = time.Now().Add(calc)
	}
	return deadline{t: t}
}

func (ch *deadliner) setTimeout(timeout time.Duration) {
	if timeout <= 0 {
		ch.timeout = 0
		return // No timeout.
	}
	for i := uint8(0); i < 64; i++ {
		calc := time.Duration(1 << i)
		if calc > timeout {
			ch.timeout = i
			return
		}
	}
}
