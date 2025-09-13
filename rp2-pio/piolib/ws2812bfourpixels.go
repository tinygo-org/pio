//go:build rp2350

package piolib

import (
	"image/color"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type WS2812bFourPixelsMode uint8

const (
	WS2812bFourPixelsModeRGB WS2812bFourPixelsMode = iota
	WS2812bFourPixelsModeRGBW
)

// WS2812bFourPixels is an RGB LED strip controller implementation, also known as NeoPixel.
type WS2812bFourPixels struct {
	sm     pio.StateMachine
	cfg    pio.StateMachineConfig
	offset uint8
	mode   WS2812bFourPixelsMode
}

// WS2812bFourPixels uses the RP2350 PIO's FJOIN_RX_GET mode to control 4 NeoPixels. The
// PIO sends color data out on repeat with no loops/buffer-filling/intervention
// of any kind needed except to change the pixel colors or brightness.
// RP2350-only due to RX FIFO register usage.
func NewWS2812bFourPixelsRGB(sm pio.StateMachine, pin machine.Pin) (*WS2812bFourPixels, error) {
	return newWS2812bFourPixels(sm, pin, WS2812bFourPixelsModeRGB)
}

func NewWS2812bFourPixelsRGBW(sm pio.StateMachine, pin machine.Pin) (*WS2812bFourPixels, error) {
	return newWS2812bFourPixels(sm, pin, WS2812bFourPixelsModeRGBW)
}

func newWS2812bFourPixels(sm pio.StateMachine, pin machine.Pin, mode WS2812bFourPixelsMode) (*WS2812bFourPixels, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.

	const pixelFreq = 800 * machine.KHz
	whole, frac, err := pio.ClkDivFromFrequency(pixelFreq*ws2812bfourpixelsCyclesPerBit, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812bfourpixelsInstructions, ws2812bfourpixelsOrigin)
	if err != nil {
		return nil, err
	}

	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)

	cfg := ws2812bfourpixelsProgramDefaultConfig(offset)
	cfg.SetSidesetPins(pin)
	cfg.SetClkDivIntFrac(whole, frac)
	cfg.SetFIFOJoin(pio.FifoJoinRxGet)

	switch mode {
	case WS2812bFourPixelsModeRGBW:
		cfg.SetOutShift(false, true, 32)
	default:
		cfg.SetOutShift(false, true, 24)
	}

	ns := &WS2812bFourPixels{
		sm:     sm,
		cfg:    cfg,
		offset: offset,
		mode:   mode,
	}
	ns.sm.Init(ns.offset, ns.cfg)
	ns.sm.SetEnabled(true)

	// Zero out the FIFO values to avoid sending residual junk data
	for i := range 4 {
		ns.SetRaw(i, 0)
	}

	return ns, nil
}

// SetEnabled starts or stops the state machine.
func (ns *WS2812bFourPixels) SetEnabled(enable bool) {
	ns.sm.SetEnabled(enable)
}

// GetMode returns the mode of the WS2812bFourPixels instance.
func (ns *WS2812bFourPixels) GetMode() WS2812bFourPixelsMode {
	return ns.mode
}

// SetRGB sets a given pixel to an RGB color value.
func (ns *WS2812bFourPixels) SetRGB(index int, r, g, b uint8) {
	ns.SetRGBW(index, r, g, b, 0)
}

// SetRGBW sets a given pixel to an RGBW color value.
func (ns *WS2812bFourPixels) SetRGBW(index int, r, g, b, w uint8) {
	// Shift occurs to left for WS2812B to interpret correctly.
	color := uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8 | uint32(w)
	ns.SetRaw(index, color)
}

// SetRaw sets a given pixel to a raw color value. The grb uint32 is a WS2812B color
// which can be created with 3 uint8 color values:
//
//	color := uint32(green)<<24 | uint32(red)<<16 | uint32(blue)<<8
func (ns *WS2812bFourPixels) SetRaw(index int, grbw uint32) {
	// Reverse the index, the PIO prints in reverse order
	fifoIndex := 3 - index
	ns.sm.SetRxFIFOAt(grbw, fifoIndex)
}

// SetColor wraps SetRGB for a [color.Color] type.
func (ns *WS2812bFourPixels) SetColor(index int, c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ns.SetRGB(index, uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
