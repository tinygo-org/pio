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

func NewWS2812(sm pio.StateMachine, pin machine.Pin) (*WS2812, error) {
	// These timings are taken from the table "Updated simplified timing
	// constraints for NeoPixel strings" at:
	// https://wp.josh.com/2014/05/13/ws2812-neopixels-are-not-so-finicky-once-you-get-to-know-them/
	// Here is a copy:
	//   Symbol   Parameter                    Min   Typical    Max   Units
	//   T0H      0 code, high voltage time    200       350    500   ns
	//   T1H      1 code, high voltage time    550       700   5500   ns
	//   TLD      data, low voltage time       450       600   5000   ns
	//   TLL      latch, low voltage time     6000                    ns
	// The equivalent table for WS2811 LEDs would be the following:
	//   Symbol   Parameter                    Min   Typical    Max   Units
	//   T0H      0 code, high voltage time    350       500    650   ns
	//   T1H      1 code, high voltage time   1050      1200   5500   ns
	//   TLD      data, low voltage time      1150      1300   5000   ns
	//   TLL      latch, low voltage time     6000                    ns
	// Combining the two (min and max) leads to the following table:
	//   Symbol   Parameter                    Min   Typical    Max   Units
	//   T0H      0 code, high voltage time    350         -    500   ns
	//   T1H      1 code, high voltage time   1050         -   5500   ns
	//   TLD      data, low voltage time      1150         -   5000   ns
	//   TLL      latch, low voltage time     6000                    ns
	// These comined timings are used so that the ws2812 package is compatible
	// with both WS2812 and with WS2811 chips.
	// T0H is the time the pin should be high to send a "0" bit.
	// T1H is the time the pin should be high to send a "1" bit.
	// TLD is the time the pin should be low between bits.
	// TLL is the time the pin should be low to apply (latch) the new colors.
	//
	// These constants are for the most part not used but serve as
	// a notebook of documentation. When in a IDE such as VSCode one
	// can mouse over constants to observe their value.
	const (
		t0h        = 352 // augment it slightly to compensate for PIO clock instability.
		t0h_cycles = 3   // Taken from PIO program by observing instructions used.
		freq       = 1_000_000_000 * t0h_cycles / t0h
		t1h_t0     = 1050. / t0h         // =3
		tld_t0     = 1150. / t0h         // ~3.8
		tll_t0     = 6000. / t0h         // ~18
		t1h_cycles = t1h_t0 * t0h_cycles // =9
		tld_cycles = tld_t0 * t0h_cycles // ~9.8
		tll_cycles = tll_t0 * t0h_cycles // ~51.42
	)
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	whole, frac, err := pio.ClkDivFromFrequency(freq, machine.CPUFrequency())
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
	// Has little endian encoding over wire, last bits are most significant.
	// So green comes first, followed by red and lastly blue.
	color := uint32(g) | uint32(r)<<8 | uint32(b)<<16
	ws.sm.TxPut(color)
}

func (ws *WS2812) SetColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.SetRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}
