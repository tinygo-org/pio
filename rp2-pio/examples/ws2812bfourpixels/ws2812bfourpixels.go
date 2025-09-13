//go:build rp2350

package main

import (
	"machine"
	"math"
	"strconv"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

var ws2812Pin string

/*
WS2812bFourPixels is a toy implementation of a NeoPixel library using the
new PIO FJOIN_RX_GET mode introduced with the RP2350.

It supports a string of up to 4 NeoPixels, using the RX FIFO to persistently
store the NeoPixel color data instead of system memory via DMA. The NeoPixels
will be continuously updated from the color data stored in the RX FIFO and can
be updated at any time using the ns.SetRGB(), ns.SetRGBW(), ns.SetRaw(), and
ns.SetColor() methods. These methods use the StateMachine.SetRxFIFOAt()
method to update the color data in the RX FIFO.

WS2812bFourPixels supports both RGB and RGBW modes, RGB using the NewWS2812bFourPixelsRGB()
function as shown here and RGBW using the NewWS2812bFourPixelsRGBW() function.

This example package can be flashed, specifying the GPIO number via the -ldflags
flag like so:
tinygo flash -target=$TARGET_NAME -ldflags "-X main.ws2812Pin=$GPIO_NUMBER" ./examples/ws2812bfourpixels/
*/
func main() {
	pinNum, err := strconv.Atoi(ws2812Pin)
	if err != nil {
		println("Invalid pin number: " + ws2812Pin)
		pinNum = 25
	}
	Pio := pio.PIO0
	sm, _ := Pio.ClaimStateMachine()
	ns, err := piolib.NewWS2812bFourPixelsRGB(sm, machine.Pin(pinNum))
	if err != nil {
		panic(err.Error())
	}

	i := 0
	stepMax := 64
	for {
		ns.SetRaw(0, getRainbowGRB(i+6%stepMax, 0.3, 64))
		ns.SetRaw(1, getRainbowGRB(i+4%stepMax, 0.3, 64))
		ns.SetRaw(2, getRainbowGRB(i+2%stepMax, 0.3, 64))
		ns.SetRaw(3, getRainbowGRB(i+0%stepMax, 0.3, 64))
		i++
		time.Sleep(100 * time.Millisecond)
	}
}

func getRainbowGRB(step int, frequency, maxValue float64) uint32 {
	// Frequency controls how fast the colors change.
	// A smaller value creates a longer, slower cycle.

	baseStep := float64(step) * frequency
	stepTwo := baseStep + math.Pi*2/3
	stepThree := baseStep + math.Pi*4/3

	// We use sine waves offset by 120 degrees (2*Pi/3 radians) for each color channel.
	// This creates a smooth transition through the color spectrum.
	red := math.Sin(baseStep)*0.5 + 0.5
	green := math.Sin(stepTwo)*0.5 + 0.5
	blue := math.Sin(stepThree)*0.5 + 0.5

	// The sine waves produce values between 0.0 and 1.0.
	// We scale these values by the desired maxValue.
	r := uint32(red * maxValue)
	g := uint32(green * maxValue)
	b := uint32(blue * maxValue)

	return g<<24 | r<<16 | b<<8
}
