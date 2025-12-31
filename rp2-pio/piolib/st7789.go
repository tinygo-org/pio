package piolib

import (
	"device/rp"
	"machine"
	"runtime/volatile"
	"time"
)

const (
	CmdSoftReset   = 0x01
	CmdSleepOut    = 0x11
	CmdDispOn      = 0x29
	CmdColAddr     = 0x2A
	CmdRowAddr     = 0x2B
	CmdRAMWrite    = 0x2C
	CmdPixelFormat = 0x3A
	CmdMADCTL      = 0x36
)

type ST7789Display struct {
	pio  *rp.PIO_Type
	sm   uint32
	w    uint16
	h    uint16
	dcPin machine.Pin
	wrPin machine.Pin
	csPin machine.Pin
}

//go:noinline
func NewST7789(width, height uint16) *ST7789Display {
	return &ST7789Display{
		pio:   rp.PIO0,
		sm:    0,
		w:     width,
		h:     height,
		dcPin: machine.GPIO8,
		wrPin: machine.GPIO9,
		csPin: machine.GPIO10,
	}
}

//go:noinline
func (d *ST7789Display) Init() error {
	d.dcPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.wrPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	d.csPin.Configure(machine.PinConfig{Mode: machine.PinOutput})

	for i := 0; i < 8; i++ {
		pin := machine.Pin(i)
		pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	d.csPin.High()
	d.wrPin.High()

	d.WriteCommand(CmdSoftReset)
	time.Sleep(150 * time.Millisecond)

	d.WriteCommand(CmdSleepOut)
	time.Sleep(100 * time.Millisecond)

	d.WriteCommand(CmdPixelFormat)
	d.WriteData(0x55)

	d.WriteCommand(CmdMADCTL)
	d.WriteData(0x00)

	d.WriteCommand(CmdDispOn)
	time.Sleep(100 * time.Millisecond)

	return nil
}

//go:noinline
func (d *ST7789Display) WriteCommand(cmd uint8) {
	d.csPin.Low()
	d.dcPin.Low()
	d.writeByte(cmd)
	d.csPin.High()
}

//go:noinline
func (d *ST7789Display) WriteData(data uint16) {
	d.csPin.Low()
	d.dcPin.High()
	d.writeByte(uint8(data >> 8))
	d.writeByte(uint8(data & 0xFF))
	d.csPin.High()
}

//go:noinline
func (d *ST7789Display) WritePixels(pixels []uint16) {
	d.csPin.Low()
	d.dcPin.High()

	for _, pixel := range pixels {
		d.writeByte(uint8(pixel >> 8))
		d.writeByte(uint8(pixel & 0xFF))
	}

	d.csPin.High()
}

//go:noinline
func (d *ST7789Display) SetWindow(xs, ys, xe, ye uint16) {
	d.WriteCommand(CmdColAddr)
	d.WriteData(xs)
	d.WriteData(xe)

	d.WriteCommand(CmdRowAddr)
	d.WriteData(ys)
	d.WriteData(ye)

	d.WriteCommand(CmdRAMWrite)
}

//go:noinline
func (d *ST7789Display) writeByte(b uint8) {
	for i := 0; i < 8; i++ {
		pin := machine.Pin(i)
		if (b & (1 << uint(7-i))) != 0 {
			pin.High()
		} else {
			pin.Low()
		}
	}

	volatile.Load((*uint32)(volatile.UnsafePointer(&rp.SIO.GPIO_OUT)))

	d.wrPin.Low()
	volatile.Load((*uint32)(volatile.UnsafePointer(&rp.SIO.GPIO_OUT)))
	d.wrPin.High()

	volatile.Load((*uint32)(volatile.UnsafePointer(&rp.SIO.GPIO_OUT)))
}
