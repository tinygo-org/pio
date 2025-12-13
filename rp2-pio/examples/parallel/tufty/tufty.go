package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

const clockHz = 133000000

// Pimoroni Tufty definitions https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GPIO10
	dcPin  = machine.GPIO11
	wrPin  = machine.GPIO12
	db0Pin = machine.GPIO14
	rdPin  = machine.GPIO13
	blPin  = machine.GPIO2
)

func main() {
	time.Sleep(5 * time.Second)
	println("Initializing Display")
	const MHz = 1_000_000
	sm, _ := pio.PIO0.ClaimStateMachine()
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

	println("Setting Up DMA")
	p8tx.EnableDMA(true)

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

	if err != nil {
		panic(err.Error())
	}
	//display.pl.Tx8([]byte("Hello World"))
	// Setup DMA
	rdPin.High()

	println("Display Common Init")
	display.CommonInit()

	blue := color.RGBA{0, 0, 255, 128}
	red := color.RGBA{255, 0, 0, 128}

	for {
		println("Making Screen Red")
		display.FillRectangle(0, 0, 320, 240, red)
		time.Sleep(2 * time.Second)

		println("Making Screen Blue")
		display.FillRectangle(0, 0, 320, 240, blue)
		time.Sleep(2 * time.Second)
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
