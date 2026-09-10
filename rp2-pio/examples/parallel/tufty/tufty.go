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

	// Configure control pins to safe idle levels BEFORE bringing up the PIO
	// parallel bus. If CS or DC are floating while the PIO state machine
	// starts and puts its initial (zeroed) OSR contents on the bus, the
	// panel intermittently latches stray bytes as commands, leaving the
	// display in an unknown state that manifests as "sometimes it doesn't
	// come up after reset".
	csPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	csPin.High() // CS idle high (panel deselected)
	dcPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dcPin.High() // DC idle in data mode
	rdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	rdPin.High() // RD held high so the panel accepts writes

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

	// Feed the PIO TX FIFO by DMA so large pixel writes do not block on the CPU.
	if err := display.pl.EnableDMA(true); err != nil {
		panic(err.Error())
	}

	display.CommonInit()

	rotations := []Rotation{Rotation0, Rotation90, Rotation180, Rotation270}
	palette := []color.RGBA{
		{255, 255, 255, 255}, // white
		{255, 0, 0, 255},     // red
		{0, 255, 0, 255},     // green
		{255, 255, 0, 255},   // yellow
	}
	black := color.RGBA{0, 0, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}

	pause := func() { time.Sleep(2 * time.Second) }

	for {
		for _, rotation := range rotations {
			// Reposition the addressing window and MADCTL for the new
			// orientation. This is much cheaper than a full CommonInit,
			// which would re-run the panel's power-on sequence.
			display.configureDisplayRotation(rotation)
			w, h := display.Size()

			// Stage 1: full screen blue fill.
			if err := display.FillRectangle(0, 0, w, h, blue); err != nil {
				panic(err.Error())
			}
			pause()

			// Stage 2: quadrant fill, white/red/green/yellow, TL/TR/BL/BR.
			hw, hh := w/2, h/2
			if err := display.FillRectangle(0, 0, hw, hh, palette[0]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(hw, 0, w-hw, hh, palette[1]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(0, hh, hw, h-hh, palette[2]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(hw, hh, w-hw, h-hh, palette[3]); err != nil {
				panic(err.Error())
			}
			pause()

			// Stage 3: four colored boxes, one per corner, on a black background.
			if err := display.FillRectangle(0, 0, w, h, black); err != nil {
				panic(err.Error())
			}
			boxW, boxH := w/6, h/6
			if err := display.FillRectangle(0, 0, boxW, boxH, palette[0]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(w-boxW, 0, boxW, boxH, palette[1]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(0, h-boxH, boxW, boxH, palette[2]); err != nil {
				panic(err.Error())
			}
			if err := display.FillRectangle(w-boxW, h-boxH, boxW, boxH, palette[3]); err != nil {
				panic(err.Error())
			}
			pause()

			// Stage 4: fill stress test. Concentric 1px rings, alternating
			// a palette color and black, shrinking the window by 2px (1px
			// per edge) each fill.
			x, y, rw, rh := int16(0), int16(0), w, h
			ci := 0
			for rw > 0 && rh > 0 {
				if err := display.FillRectangle(x, y, rw, rh, palette[ci%len(palette)]); err != nil {
					panic(err.Error())
				}
				ci++
				x, y, rw, rh = x+1, y+1, rw-2, rh-2
				if rw <= 0 || rh <= 0 {
					break
				}
				if err := display.FillRectangle(x, y, rw, rh, black); err != nil {
					panic(err.Error())
				}
				x, y, rw, rh = x+1, y+1, rw-2, rh-2
			}
			pause()
		}
	}
}

type Displayer interface {
	// Size returns the current size of the display.
	Size() (x, y int16)

	// SetPixel modifies the internal buffer.
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
