//go:build tufty2040

package main

import "machine"

// Pimoroni Tufty 2040 LCD pin definitions.
// https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GPIO10 // LCD_CS
	dcPin  = machine.GPIO11 // LCD_DC
	wrPin  = machine.GPIO12 // LCD_WR
	rdPin  = machine.GPIO13 // LCD_RD
	db0Pin = machine.GPIO14 // LCD_DB0..DB7 = GPIO14..GPIO21
	blPin  = machine.GPIO2  // LCD_BACKLIGHT
)

// busBaud controls the PIO parallel bus clock rate driving WR.
//
// The parallel PIO program in piolib is three instructions long, so the PIO
// state machine clock runs at 3 * busBaud. The ST7789 8080-II parallel
// interface specifies a minimum write cycle of 66 ns (~15.15 MHz), and
// 15 MHz has been verified visually clean on a Tufty 2040 panel.
const busBaud = 15_000_000

func configureBoard() {}

func configureBacklight(pin machine.Pin, on bool) error {
	return configureBacklightPWM(machine.PWM1, pin, on)
}
