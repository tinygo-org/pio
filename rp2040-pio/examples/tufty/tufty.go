package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

const clockHz = 133000000

// Pimoroni Tufty definitions https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GP10
	dcPin  = machine.GP11
	wrPin  = machine.GP12
	db0Pin = machine.GP14
	rdPin  = machine.GP13
	blPin  = machine.GP2
)

func main() {
	time.Sleep(5 * time.Second)
	println("Initializing Display")
	display := ST7789{
		cs:       csPin,
		dc:       dcPin,
		rd:       rdPin,
		bl:       blPin,
		width:    320,
		height:   240,
		rotation: Rotation0,
	}

	println("Initializing PIO")
	println("Parallel Init")
	err := display.ParallelInit(pio.PIO0.StateMachine(0), db0Pin, wrPin)
	if err != nil {
		panic(err.Error())
	}
	display.pl.Write([]byte("Hello World"))
	// Setup DMA
	println("Setting Up DMA")
	// display.pl.EnableDMA(2)
	rdPin.High()

	println("Display Common Init")
	display.CommonInit()

	println("Making Screen Blue")
	blue := color.RGBA{255, 255, 255, 255}
	display.FillRectangle(0, 0, 320, 240, blue)
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
