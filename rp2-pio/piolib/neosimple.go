//go:build rp2350

package piolib

import (
	"image/color"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type NeoSimpleMode uint8

const (
	NeoSimpleModeRGB NeoSimpleMode = iota
	NeoSimpleModeRGBW
)

// NeoSimple is an RGB LED strip controller implementation, also known as NeoPixel.
type NeoSimple struct {
	sm     pio.StateMachine
	cfg    pio.StateMachineConfig
	offset uint8
	mode   NeoSimpleMode
}

// NeoSimple uses the RP2350 PIO's FJOIN_RX_GET mode to control 4 NeoPixels. The
// PIO sends color data out on repeat with no loops/buffer-filling/intervention
// of any kind needed except to change the pixel colors or brightness.
// RP2350-only due to RX FIFO register usage.
func NewNeoSimpleRGB(sm pio.StateMachine, pin machine.Pin) (*NeoSimple, error) {
	return newNeoSimple(sm, pin, NeoSimpleModeRGB)
}

func NewNeoSimpleRGBW(sm pio.StateMachine, pin machine.Pin) (*NeoSimple, error) {
	return newNeoSimple(sm, pin, NeoSimpleModeRGBW)
}

func newNeoSimple(sm pio.StateMachine, pin machine.Pin, mode NeoSimpleMode) (*NeoSimple, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.

	const pixelFreq = 800 * machine.KHz
	whole, frac, err := pio.ClkDivFromFrequency(pixelFreq*neosimpleCyclesPerBit, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(neosimpleInstructions, neosimpleOrigin)
	if err != nil {
		return nil, err
	}

	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)

	cfg := neosimpleProgramDefaultConfig(offset)
	cfg.SetSidesetPins(pin)
	cfg.SetClkDivIntFrac(whole, frac)
	cfg.SetFIFOJoin(pio.FifoJoinRxGet)

	switch mode {
	case NeoSimpleModeRGBW:
		cfg.SetOutShift(false, true, 32)
	default:
		cfg.SetOutShift(false, true, 24)
	}

	ns := &NeoSimple{
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
func (ns *NeoSimple) SetEnabled(enable bool) {
	ns.sm.SetEnabled(enable)
}

// GetMode returns the mode of the NeoSimple instance.
func (ns *NeoSimple) GetMode() NeoSimpleMode {
	return ns.mode
}

// SetRGB sets a given pixel to an RGB color value.
func (ns *NeoSimple) SetRGB(index int, r, g, b uint8) {
	ns.SetRGBW(index, r, g, b, 0)
}

// SetRGBW sets a given pixel to an RGBW color value.
func (ns *NeoSimple) SetRGBW(index int, r, g, b, w uint8) {
	// Shift occurs to left for WS2812B to interpret correctly.
	color := uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8 | uint32(w)
	ns.SetRaw(index, color)
}

// SetRaw sets a given pixel to a raw color value. The grb uint32 is a WS2812B color
// which can be created with 3 uint8 color values:
//
//	color := uint32(green)<<24 | uint32(red)<<16 | uint32(blue)<<8
func (ns *NeoSimple) SetRaw(index int, grbw uint32) {
	// Reverse the index, the PIO prints in reverse order
	fifoIndex := 3 - index
	ns.sm.SetRxFIFOAt(grbw, fifoIndex)
}

// SetColor wraps SetRGB for a [color.Color] type.
func (ns *NeoSimple) SetColor(index int, c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ns.SetRGB(index, uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
