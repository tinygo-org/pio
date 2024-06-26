package main

import (
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
	const lightIntensity = 64
	rawred := rawcolor(lightIntensity, 0, 0)
	rawgreen := rawcolor(0, lightIntensity, 0)
	// Make Christmas lights of first part of strip.
	ws.EnableDMA(true)
	ws.WriteRaw([]uint32{
		0,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
		rawred,
		rawgreen,
	})

	// And sweep first LED.
	const sweepIncrement = 1
	const sweepPeriod = time.Second / 4
	for {
		println("red sweep")
		for r := uint8(0); r < 255; r += sweepIncrement {
			ws.PutRGB(r, 0, 0)
			time.Sleep(sweepPeriod)
		}
		time.Sleep(time.Second)
		println("green sweep")
		for g := uint8(0); g < 255; g += sweepIncrement {
			ws.PutRGB(0, g, 0)
			time.Sleep(sweepPeriod)
		}
		time.Sleep(time.Second)
		println("blue sweep")
		for b := uint8(0); b < 255; b += sweepIncrement {
			ws.PutRGB(0, 0, b)
			time.Sleep(sweepPeriod)
		}
		time.Sleep(time.Second)
	}
}

func rawcolor(r, g, b uint8) uint32 {
	return uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
}
