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
	ws, err := piolib.NewWS2812B(sm, ws2812Pin)
	if err != nil {
		panic(err.Error())
	}
	const maxVal = 4
	red := color.RGBA{R: maxVal}
	amber := color.RGBA{R: maxVal, G: maxVal / 4 * 3}
	green := color.RGBA{G: maxVal}
	for {
		for g := uint8(0); g < 255; g |= 1 {
			for r := uint8(0); r < 255; r |= 1 {
				for b := uint8(0); b < 255; b |= 1 {
					time.Sleep(time.Second / 8)
					ws.SetRGB(r, g, b)
					b <<= 1
					println(r, g, b)
				}
				println("max blue")
				r <<= 1
			}
			println("max red")
			g <<= 1
		}
		println("max greenb")
	}

	// Stoplight sequence.
	for {
		const longWait = 6 * time.Second
		const shortWait = 2 * time.Second
		// Start Stoplight in red (STOP).
		println("red")
		ws.SetColor(red)
		time.Sleep(4 * time.Second)

		// Before green we go through a red+yellow stage (PREP. PULL AWAY)
		println("green/amber switching")
		for i := 0; i < 2; i++ {
			const semiSleep = time.Second / 2
			ws.SetColor(amber)
			time.Sleep(semiSleep)
			ws.SetColor(red)
			time.Sleep(semiSleep)
		}
		println("green")
		ws.SetColor(green)
		time.Sleep(longWait)

		println("amber")
		ws.SetColor(amber)
		time.Sleep(shortWait)
	}
}
