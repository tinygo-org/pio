//go:build rp2040

package piolib

import (
	"image/color"
	"machine"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

type WS2812 struct {
	sm     pio.StateMachine
	offset uint8
}

func NewWS2812(sm pio.StateMachine, pin machine.Pin, nbits uint8, baud uint32) (*WS2812, error) {
	// We add the program to PIO memory and store it's offset.
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812Instuctions, ws2812Origin)
	if err != nil {
		return nil, err
	}

	cfg := ws2812ProgramDefaultConfig(offset)

	// Configure sideset pin.
	cfg.SetSetPins(pin, 1)
	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)

	cfg.SetOutShift(false, true, uint16(nbits))

	// We only use Tx FIFO, so we set the join to Tx.
	cfg.SetFIFOJoin(pio.FifoJoinTx)

	// Configure Baud if non-zero. else use default
	if baud != 0 {
		baud *= 8 // 8 PIO instructions per bit.
		whole, frac, err := pio.ClkDivFromPeriod(1e9/baud, machine.CPUFrequency())
		if err != nil {
			return nil, err
		}
		cfg.SetClkDivIntFrac(whole, frac)
	}

	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	dev := &WS2812{sm: sm, offset: offset}
	return dev, nil
}

func (ws *WS2812) setPixels(pixels []byte) {
	for _, pixel := range pixels {
		ws.sm.TxPut(uint32(pixel) << 24)
	}
}

func (ws *WS2812) SetRGB(r, g, b uint8) {
	ws.setPixels([]byte{r, g, b})
	// color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	println("r", r, "g", g, "b", b)

}

func (ws *WS2812) SetColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.SetRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}

const ws2812Origin = -1

var ws2812Instuctions = []uint16{
	//     .wrap_target
	0x6221, //  0: out    x, 1            side 0 [2]
	0x1123, //  1: jmp    !x, 3           side 1 [1]
	0x1400, //  2: jmp    0               side 1 [4]
	0xa442, //  3: nop                    side 0 [4]
	//     .wrap
}

func ws2812ProgramDefaultConfig(offset uint8) pio.StateMachineConfig {
	// https://github.com/adafruit/Adafruit_NeoPixel/blob/fe882b84951bed066764f9350e600a2ec2aa5a9e/rp2040_pio.h#L23
	const ledWrapTarget = 0
	const ledWrap = 3
	cfg := pio.DefaultStateMachineConfig()
	cfg.SetWrap(offset+ledWrapTarget, offset+ledWrap)
	cfg.SetSidesetParams(1, false, false)
	return cfg
}
