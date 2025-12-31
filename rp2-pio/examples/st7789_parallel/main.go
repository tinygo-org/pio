package main

import (
	"machine"
	"time"

	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

//go:noinline
func main() {
	display := piolib.NewST7789(240, 320)
	display.Init()
	display.SetWindow(0, 0, 239, 319)

	pixels := make([]uint16, 240*320)
	for i := range pixels {
		pixels[i] = 0xFFFF
	}

	display.WritePixels(pixels)

	for {
		time.Sleep(time.Second)
	}
}
