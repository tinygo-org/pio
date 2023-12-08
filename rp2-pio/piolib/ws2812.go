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
		t1l                 = 600           // 400ns
		t1lcycles           = 6             // 3 cycles per t0h, from pio file.
		f1l                 = nanosecondsInSecond / (t1l / t1lcycles)
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
	dev.SetT1L(t1l)
	return dev, nil
}

// SetT1L sets the period of the T0H pulse.
func (ws *WS2812) SetT1L(d time.Duration) error {
	const t1lcycles = 6
	d /= t1lcycles
	whole, frac, err := pio.ClkDivFromPeriod(uint32(d), machine.CPUFrequency())
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
