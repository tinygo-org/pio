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
		Width:    320,
		Height:   240,
		Rotation: st7789.NO_ROTATION,
	})

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

// func (st *ST7789) command(command byte, data []byte) {
// 	st.dc.Low()
// 	st.cs.Low()
// 	st.pl.Tx8([]byte{command})
// 	// st.writeBlockingParallel([]byte{command}, 1)

// 	if len(data) > 0 {
// 		st.dc.High()
// 		st.pl.Tx8(data)
// 		// st.writeBlockingParallel(data, len(data))
// 	}
// 	st.cs.High()
// }
