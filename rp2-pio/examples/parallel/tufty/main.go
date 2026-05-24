package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Pimoroni Tufty 2040 pin definitions
// https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GP10
	dcPin  = machine.GP11
	wrPin  = machine.GP12
	db0Pin = machine.GP14
	rdPin  = machine.GP13
	blPin  = machine.GP2
)

func main() {
	time.Sleep(3 * time.Second)
	println("Initializing ST7789 Display via PIO parallel interface")

	const MHz = 1_000_000

	// Claim a PIO state machine
	sm, _ := pio.PIO0.ClaimStateMachine()

	// Create ST7789 display driver with PIO parallel bus
	//
	// Key configuration for ST7789 8080 parallel protocol:
	// - BusWidth=8: 8-bit parallel data bus (D0-D7)
	// - BitsPerPull=8: Auto-pull after 8 bits (one byte per FIFO word)
	// - ShiftLeft=true: Correct bit-to-pin mapping (bit 0 -> pin D0)
	// - FastMode=true: 2-instruction PIO program for higher throughput
	// - Baud=1MHz: Display clock rate
	//
	// The PIO program satisfies the ST7789 V_m timing constraint:
	// data is output on pins before the write clock (WRX) goes HIGH.
	display, err := piolib.NewST7789(sm, piolib.ParallelConfig{
		Baud:        1 * MHz,
		Clock:       wrPin,
		DataBase:    db0Pin,
		BusWidth:    8,
		BitsPerPull: 8,
		ShiftLeft:   true,
		FastMode:    true,
	}, piolib.ST7789Config{
		Width:  320,
		Height: 240,
		CS:     csPin,
		DC:     dcPin,
		RD:     rdPin,
		BL:     blPin,
	})
	if err != nil {
		panic(err.Error())
	}

	// Initialize display with standard ST7789 sequence
	display.Initialize()

	// Apply RAM control fix to eliminate horizontal banding
	// This is the critical fix for displays like the Tufty 2040
	display.ApplyRAMControlFix()

	// Display test pattern: cycle through colors
	for {
		println("Red")
		display.FillScreen(color565(255, 0, 0))
		time.Sleep(500 * time.Millisecond)

		println("Green")
		display.FillScreen(color565(0, 255, 0))
		time.Sleep(500 * time.Millisecond)

		println("Blue")
		display.FillScreen(color565(0, 0, 255))
		time.Sleep(500 * time.Millisecond)

		println("White")
		display.FillScreen(color565(255, 255, 255))
		time.Sleep(500 * time.Millisecond)

		println("Black")
		display.FillScreen(color565(0, 0, 0))
		time.Sleep(500 * time.Millisecond)
	}
}

// color565 converts RGB (0-255 each) to RGB565 format.
func color565(r, g, b uint8) uint16 {
	return uint16((uint16(r)&0xF8)<<8 |
		(uint16(g)&0xFC)<<3 |
		uint16(b)>>3)
}

// RGBATo565 converts an RGBA color to RGB565 format.
func RGBATo565(c color.RGBA) uint16 {
	r, g, b, _ := c.RGBA()
	return uint16((r & 0xF800) +
		((g & 0xFC00) >> 5) +
		((b & 0xF800) >> 11))
}
