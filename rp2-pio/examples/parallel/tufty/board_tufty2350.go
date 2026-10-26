//go:build tufty2350 || pico_plus2

package main

import (
	"machine"
	"time"
)

// Pimoroni Tufty 2350 LCD pin definitions.
// Use -target=tufty2350 with TinyGo 0.43.0 or later.
// The pico-plus2 target is also supported for older TinyGo versions.
const (
	csPin    = machine.GPIO27 // LCD_CS
	dcPin    = machine.GPIO28 // LCD_RS
	wrPin    = machine.GPIO30 // LCD_WR
	rdPin    = machine.GPIO31 // LCD_RD
	db0Pin   = machine.GPIO32 // LCD_DB0..DB7 = GPIO32..GPIO39
	blPin    = machine.GPIO26 // LCD_BACKLIGHT
	powerPin = machine.GPIO41 // POWER_EN
)

// busBaud controls the PIO parallel bus clock rate driving WR.
//
// The RP2350 default 150 MHz sysclk produces a 15.006 MHz WR rate
// (66.64 ns write cycle), within the ST7789 8080-II timing limit.
const busBaud = 15_000_000

func configureBoard() {
	powerPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	powerPin.High()
	time.Sleep(500 * time.Millisecond)
}

func configureBacklight(pin machine.Pin, on bool) error {
	return configureBacklightPWM(machine.PWM5, pin, on)
}
