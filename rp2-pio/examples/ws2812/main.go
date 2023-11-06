package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

func main() {
	const ws2812Pin = machine.GP16
	sm, _ := pio.PIO0.ClaimStateMachine()
	ws, err := piolib.NewWS2812(sm, ws2812Pin, 400_000)
	if err != nil {
		panic(err.Error())
	}
	const maxVal = 255
	red := color.RGBA{R: maxVal}
	amber := color.RGBA{R: maxVal, G: maxVal / 4 * 3}
	green := color.RGBA{G: maxVal}

	// Stoplight sequence.
	for {
		const longWait = 6 * time.Second
		const shortWait = 2 * time.Second
		// Start Stoplight in red (STOP).
		ws.SetColor(red)
		time.Sleep(4 * time.Second)

		// Before green we go through a red+yellow stage (PREP. PULL AWAY)
		for i := 0; i < 2; i++ {
			const semiSleep = time.Second / 2
			ws.SetColor(amber)
			time.Sleep(semiSleep)
			ws.SetColor(red)
			time.Sleep(semiSleep)
		}
		ws.SetColor(green)
		time.Sleep(longWait)
		ws.SetColor(amber)
		time.Sleep(shortWait)
	}
}
