//go:build rp2040

package piolib

import (
	"image/color"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type WS2812 struct {
	sm     pio.StateMachine
	offset uint8
}

func NewWS2812(sm pio.StateMachine, pin machine.Pin, baud uint32) (*WS2812, error) {
	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
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
	cfg.SetClkDivIntFrac(whole, frac)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	dev := &WS2812{sm: sm, offset: offset}
	return dev, nil
}

func (ws *WS2812) SetRGB(r, g, b uint8) {
	color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	println("r", r, "g", g, "b", b)
	ws.sm.TxPut(color)
}

func (ws *WS2812) SetColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.SetRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
