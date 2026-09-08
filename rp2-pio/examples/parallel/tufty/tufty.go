package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Pimoroni Tufty definitions https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GPIO10 // LCD_CS
	dcPin  = machine.GPIO11 // LCD_DC
	wrPin  = machine.GPIO12 // LCD_WR
	db0Pin = machine.GPIO14 // LCD_DB0..DB7 = GPIO14..GPIO21
	rdPin  = machine.GPIO13 // LCD_RD
	blPin  = machine.GPIO2  // LCD_BACKLIGHT
)

func main() {
	time.Sleep(5 * time.Second) // wait for the USB CDC console to enumerate

	const MHz = 1_000_000
	sm, _ := pio.PIO0.ClaimStateMachine()

	// Drive the 8 bit parallel bus from PIO, clocking data out on WR.
	p8tx, err := piolib.NewParallel(sm, piolib.ParallelConfig{
		Baud:        1 * MHz,
		Clock:       wrPin,
		DataBase:    db0Pin,
		BusWidth:    8,
		BitsPerPull: 8,
	})
	if err != nil {
		panic(err.Error())
	}
	display := ST7789{
		pl:       p8tx,
		cs:       csPin,
		dc:       dcPin,
		rd:       rdPin,
		bl:       blPin,
		width:    320,
		height:   240,
		rotation: Rotation0,
	}

	// RD must be configured as an output and held high (deasserted) for the
	// ST7789 to accept writes on the parallel bus.
	rdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	rdPin.High()

	// Feed the PIO TX FIFO by DMA so large pixel writes do not block on the CPU.
	if err := display.pl.EnableDMA(true); err != nil {
		panic(err.Error())
	}

	display.CommonInit()

	blue := color.RGBA{0, 0, 255, 255}
	if err := display.FillRectangle(0, 0, 320, 240, blue); err != nil {
		panic(err.Error())
	}
}

type Displayer interface {
	// Size returns the current size of the display.
	Size() (x, y int16)

	// SetPizel modifies the internal buffer.
	SetPixel(x, y int16, c color.RGBA)

	// Display sends the buffer (if any) to the screen.
	Display() error
}

// Rotation is how much a display has been rotated. Displays can be rotated, and
// sometimes also mirrored.
type Rotation uint8

// Clockwise rotation of the screen.
const (
	Rotation0 = iota
	Rotation90
	Rotation180
	Rotation270
	Rotation0Mirror
	Rotation90Mirror
	Rotation180Mirror
	Rotation270Mirror
)
