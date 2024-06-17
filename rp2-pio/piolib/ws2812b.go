//go:build rp2040

package piolib

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type WS2812B struct {
	sm     pio.StateMachine
	offset uint8
}

func NewWS2812B(sm pio.StateMachine, pin machine.Pin) (*WS2812B, error) {
	time.Sleep(2 * time.Second)
	// https://cdn-shop.adafruit.com/datasheets/WS2812B.pdf
	const (
		t0h        = 410 // augment it slightly to compensate for PIO clock instability.
		t0h_cycles = 3   // Taken from PIO program by observing instructions used.
		period     = t0h / t0h_cycles
		freq       = 1_000_000_000 / period
	)
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	cpufreq := machine.CPUFrequency()
	// whole, frac, err := pio.ClkDivFromPeriod(period, cpufreq)
	whole, frac, err := pio.ClkDivFromFrequency(freq, cpufreq)
	if err != nil {
		return nil, err
	}
	// We add the program to PIO memory and store it's offset.
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812b_ledInstructions, ws2812b_ledOrigin)
	if err != nil {
		return nil, err
	}
	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)
	cfg := ws2812b_ledProgramDefaultConfig(offset)
	cfg.SetSetPins(pin, 1)
	// We only use Tx FIFO, so we set the join to Tx.
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	cfg.SetClkDivIntFrac(whole, frac)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	dev := &WS2812B{sm: sm, offset: offset}
	return dev, nil
}

func (ws *WS2812B) SetRGB(r, g, b uint8) {
	// Has little endian encoding over wire, last bits are most significant.
	// So green comes first, followed by red and lastly blue.
	color := uint32(g) | uint32(r)<<8 | uint32(b)<<16
	ws.sm.TxPut(color)
}

func (ws *WS2812B) SetColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.SetRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
