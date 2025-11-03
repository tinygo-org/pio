package main

import (
	"machine"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

func main() {
	time.Sleep(time.Second)

	// p6.SetEnabled(false)
	s := &screen{
		rd0: makepinout(0),
		gd0: makepinout(1),
		bd0: makepinout(2),
		rd1: makepinout(3),
		gd1: makepinout(4),
		bd1: makepinout(5),
		clk: makepinout(6),
		lat: makepinout(7),
		oe:  makepinout(8),
	}
	sm, _ := pio.PIO0.ClaimStateMachine()
	p6, err := piolib.NewParallel6(sm, 12_000, 0, 6)
	if err != nil {
		panic(err)
	}
	err = p6.EnableDMA(true)
	if err != nil {
		panic(err)
	}
	s.p6 = p6
	s.clear()
	for {
		// testScreen(s)
		testScreenBus(s)
	}
}

// setScreen sets a 3-bit RGB value at (ix, iy).
// ix: 0..15, iy: 0..7
func setScreen(screen *[4][4]uint32, ix, iy int, rgb3bits uint8) {
	if ix < 0 || ix >= 16 || iy < 0 || iy >= 8 {
		return // or panic
	}
	colBlock := ix / 4 // 0..3
	rowIdx := iy % 4   // 0..3 within the half
	up := 1 - (iy / 4) // 0 = lower half, 1 = upper half

	groupBase := (ix % 4) * 6 // 6 bits per group
	bitoff := groupBase + up*3

	mask := uint32(0b111) << bitoff
	v := &screen[colBlock][rowIdx]
	*v &^= mask
	*v |= uint32(rgb3bits&0x7) << bitoff
}

type screen struct {
	// data pins.
	rd0, gd0, bd0, rd1, gd1, bd1 pinout
	clk                          pinout
	lat                          pinout
	oe                           pinout
	p6                           *piolib.Parallel6
}

func testScreen(s *screen) {
	var screen [4][4]uint32
	s.clear()
	for iy := 0; iy < 8; iy++ {
		for ix := 0; ix < 16; ix++ {
			setScreen(&screen, ix, iy, 0b101)
			s.draw24(screen)
			time.Sleep(30 * time.Millisecond)
		}
	}
	for range 2 {
		s.draw24(screen)
		time.Sleep(time.Second / 3)
		s.clear()
		time.Sleep(time.Second / 3)
	}
	for i := range screen {
		for j := range screen[i] {
			screen[i][j] = 0xffff_ffff
		}
	}
	s.draw24(screen)
	time.Sleep(2 * time.Second)
}

func testScreenBus(s *screen) {
	var screen [4][4]uint32
	buf := unsafe.Slice(&screen[0][0], 4*4)
	for j := range buf {
		for i := 0; i < 24; i++ {
			mask := uint32(1) << i
			buf[j] = mask
			if i >= 24 {
				println("off screen")
			}
			buf[0] |= 1
			buf[len(buf)-1] |= 1 << 23
			s.draw24(screen)
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (s *screen) latch() {
	s.lat.High()
	time.Sleep(10 * time.Microsecond)
	s.lat.Low()
}

func (s *screen) clear() {
	println("clear")
	for range 16 {
		s.clkout24(0)
	}
	s.latch()
}

func (s *screen) enable(b bool) {
	println("enable", b)
	s.oe(!b)
}

func (s *screen) draw24(screen [4][4]uint32) {
	buf := unsafe.Slice(&screen[0][0], 4*4)
	if s.p6 != nil && s.p6.IsEnabled() {
		s.p6.Tx24(buf)
		s.latch()
		return
	}
	for i := range buf {
		s.clkout24(buf[i])
	}
	s.latch()
}

func (s *screen) clkout24(v uint32) {
	const bits6 = 0b11_1111
	s.rawclkout6(uint8(v & bits6))
	s.rawclkout6(uint8((v >> 6) & bits6))
	s.rawclkout6(uint8((v >> 12) & bits6))
	s.rawclkout6(uint8((v >> 18) & bits6))
}

func (s *screen) rawclkout6(rgbBits uint8) {
	s.clk.Low()
	s.rd0(rgbBits&(1<<0) != 0)
	s.gd0(rgbBits&(1<<1) != 0)
	s.bd0(rgbBits&(1<<2) != 0)

	s.rd1(rgbBits&(1<<3) != 0)
	s.gd1(rgbBits&(1<<4) != 0)
	s.bd1(rgbBits&(1<<5) != 0)
	s.clk.High()
	s.clk.Low()
}

type pinout func(level bool)

func (p pinout) High() { p(true) }
func (p pinout) Low()  { p(false) }

func makepinout(pin machine.Pin) pinout {
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	return pin.Set
}
