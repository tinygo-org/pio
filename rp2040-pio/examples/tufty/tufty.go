package main

import (
	"image/color"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
	"tinygo.org/x/drivers"
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
		cs:                csPin,
		dc:                dcPin,
		wr:                wrPin,
		rd:                rdPin,
		d0:                db0Pin,
		bl:                blPin,
		stateMachineIndex: 0,
		dmaChannel:        2,
		width:             320,
		height:            240,
		rotation:          drivers.Rotation0,
	}

	println("Initializing PIO")
	display.pio = pio.PIO0
	println("Parallel Init")
	display.ParallelInit()

	// Setup DMA
	println("Setting Up DMA")
	dmaConfig := getDefaultDMAConfig(display.dmaChannel)
	setTransferDataSize(dmaConfig, DMA_SIZE_8)
	setBSwap(dmaConfig, false)
	setDREQ(dmaConfig, display.pio.HW().GetIRQ())
	dmaChannelConfigure(display.dmaChannel, dmaConfig, display.pio.HW().TXF0.Reg, 0, 0, false)

	rdPin.High()

	println("Display Common Init")
	display.CommonInit()

	println("Making Screen Blue")
	blue := color.RGBA{255, 255, 255, 255}
	display.FillRectangle(0, 0, 320, 240, blue)
}
