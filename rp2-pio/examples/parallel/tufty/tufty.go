package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
	"tinygo.org/x/drivers/st7789"
)

const clockHz = 133000000

// Pimoroni Tufty definitions https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin    = machine.GPIO10
	dcPin    = machine.GPIO11
	wrPin    = machine.GPIO12
	db0Pin   = machine.GPIO14
	resetPin = machine.GPIO13
	blPin    = machine.GPIO2
)

var (
	red   = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	blue  = color.RGBA{R: 0, G: 0, B: 255, A: 255}
	green = color.RGBA{R: 0, G: 255, B: 0, A: 255}
)

// sendRawCommand sends a raw command to the ST7789 display.
// This bypasses the st7789 driver to send missing initialization commands
// that Pimoroni includes but TinyGo's driver doesn't.
//
//go:noinline
func sendRawCommand(bus *piolib.Parallel, dc, cs machine.Pin, cmd byte, data []byte) {
	cs.Low()
	dc.Low() // Command mode
	bus.Tx8([]byte{cmd})
	if len(data) > 0 {
		dc.High() // Data mode
		bus.Tx8(data)
	}
	cs.High()
}

func main() {
	time.Sleep(3 * time.Second)

	println("Initializing Display")
	const MHz = 1_000_000
	sm, _ := pio.PIO0.ClaimStateMachine()
	p8tx, err := piolib.NewParallel(sm, piolib.ParallelConfig{
		Baud:        16 * MHz,
		Clock:       wrPin,
		DataBase:    db0Pin,
		BusWidth:    8,
		BitsPerPull: 8,
		FastMode:    true,
		ShiftRight:  true,
	})
	if err != nil {
		panic(err.Error())
	}

	println("Setting Up DMA")
	p8tx.EnableDMA(true)

	display := st7789.New(&parallelBus{bus: p8tx},
		resetPin, // TFT_RESET
		dcPin,    // TFT_DC
		csPin,    // TFT_CS
		blPin)    // TFT_LITE

	println("Configuring display")
	display.Configure(st7789.Config{
		Width:            320,
		Height:           240,
		Rotation:         st7789.NO_ROTATION,
		IdleModePorch:    0x33,
		PartialModePorch: 0x33,
	})

	// Apply Pimoroni's initialization commands that TinyGo's ST7789 driver is missing.
	// These are critical for proper display operation on the Tufty 2040.
	println("Applying Pimoroni init sequence...")

	// RAMCTRL (0xB0) - Pimoroni explicitly calls this the "banding fix"
	// This is the most important missing command for eliminating horizontal banding.
	sendRawCommand(p8tx, dcPin, csPin, 0xB0, []byte{0x00, 0xC0})

	// // Power and voltage control registers for stable operation
	sendRawCommand(p8tx, dcPin, csPin, 0xC0, []byte{0x2C})       // LCMCTRL - LCM control
	sendRawCommand(p8tx, dcPin, csPin, 0xC2, []byte{0x01})       // VDVVRHEN - VDV/VRH enable
	sendRawCommand(p8tx, dcPin, csPin, 0xC3, []byte{0x12})       // VRHS - VRH voltage setting
	sendRawCommand(p8tx, dcPin, csPin, 0xC4, []byte{0x20})       // VDVS - VDV voltage setting
	sendRawCommand(p8tx, dcPin, csPin, 0xD0, []byte{0xA4, 0xA1}) // PWCTRL1 - Power control

	// // PORCTRL (0xB2) - Porch timing with Pimoroni's values
	// sendRawCommand(p8tx, dcPin, csPin, 0xB2, []byte{0x0C, 0x0C, 0x00, 0x33, 0x33})

	width, height := int16(320), int16(240)
	for {
		println("Making Screen Red")
		display.FillRectangle(0, 0, width, height, red)
		time.Sleep(1 * time.Second)

		println("Making Screen Blue")
		display.FillRectangle(0, 0, width, height, blue)
		time.Sleep(1 * time.Second)
	}
}

type parallelBus struct {
	bus *piolib.Parallel
}

func (p *parallelBus) Tx(w, r []byte) error {
	return p.bus.Tx8(w)
}

func (p *parallelBus) Transfer(w byte) (byte, error) {
	p.bus.Tx8([]byte{w})

	return 0, nil
}
