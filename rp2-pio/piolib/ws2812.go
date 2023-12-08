//go:build rp2040

package piolib

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type WS2812 struct {
	sm     pio.StateMachine
	offset uint8
}

func NewWS2812(sm pio.StateMachine, pin machine.Pin) (*WS2812, error) {
	const (
		nanosecondsInSecond = 1_000_000_000 // 1e9
		t0h                 = 400           // 400ns
		t0hcycles           = 3             // 3 cycles per t0h, from pio file.
		f0h                 = nanosecondsInSecond / (t0h / t0hcycles)
	)
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	// We add the program to PIO memory and store it's offset.
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812_ledInstructions, ws2812_ledOrigin)
	if err != nil {
		return nil, err
	}
	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)
	cfg := ws2812_ledProgramDefaultConfig(offset)
	cfg.SetSetPins(pin, 1)
	// We only use Tx FIFO, so we set the join to Tx.
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	dev := &WS2812{sm: sm, offset: offset}
	dev.SetT0H(t0h)
	return dev, nil
}

// SetT0H sets the period of the T0H pulse.
func (ws *WS2812) SetT0H(d time.Duration) error {
	f0h := uint32(1_000_000_000 / d)
	whole, frac, err := pio.ClkDivFromFrequency(f0h, machine.CPUFrequency())
	if err != nil {
		return err
	}
	ws.sm.SetClkDiv(whole, frac)
	return nil
}

func (ws *WS2812) SetRGB(r, g, b uint8) {
	// WS2812 uses GRB order.
	color := uint32(g) | uint32(r)<<8 | uint32(b)<<16
	for ws.sm.IsTxFIFOFull() {
		gosched()
	}
	ws.sm.TxPut(color)
}

func (ws *WS2812) SetColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.SetRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
