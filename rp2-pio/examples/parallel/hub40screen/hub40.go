package main

import (
	"machine"
	"runtime"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

func main() {
	// sleep to manage to visualize output.
	time.Sleep(2 * time.Second)
	const usePIO = true
	var p6 Parallel6Bus
	if usePIO {
		println("use pio")
		sm, _ := pio.PIO0.ClaimStateMachine()
		p6concrete, err := piolib.NewParallel(sm, piolib.ParallelConfig{
			Baud:        12_000,
			DataBase:    machine.GPIO0,
			BusWidth:    6,
			Clock:       machine.GPIO6,
			BitsPerPull: 24,
		})
		if err != nil {
			panic(err)
		}
		err = p6concrete.EnableDMA(true)
		if err != nil {
			panic(err)
		}
		p6 = &parallel6{Parallel: *p6concrete}
	} else {
		p6 = &bitbangParallel6{
			rd0: makepinout(0),
			gd0: makepinout(1),
			bd0: makepinout(2),
			rd1: makepinout(3),
			gd1: makepinout(4),
			bd1: makepinout(5),
			clk: makepinout(6),
		}
	}
	var emptyScreen [4][4]uint32
	s := NewHUB40Screen(p6, makepinout(7), makepinout(8))
	s.Draw(&emptyScreen)
	for {
		// testScreen(s)
		testScreenBus(s)
	}
}

type parallel6 struct {
	piolib.Parallel
}

func (p6 *parallel6) Tx24(b []uint32) error {
	return p6.Parallel.Tx32(b) // parallel bus already configured as 24 shift bits.
}

// SetScreen sets a 3-bit RGB value at (ix, iy).
// ix: 0..15, iy: 0..7
func SetScreen(screen *[4][4]uint32, ix, iy int, rgb3bits uint8) {
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

type Parallel6Bus interface {
	Tx24(buf []uint32) error
}

func NewHUB40Screen(bus Parallel6Bus, latch, outputEnable pinout) *HUB40Screen {
	return &HUB40Screen{
		lat: latch,
		oe:  outputEnable,
		p6:  bus,
	}
}

func testScreen(s *HUB40Screen) {
	var emptyscreen [4][4]uint32
	var screen [4][4]uint32
	s.Draw(&emptyscreen)
	for iy := 0; iy < 8; iy++ {
		for ix := 0; ix < 16; ix++ {
			SetScreen(&screen, ix, iy, 0b101)
			s.Draw(&screen)
			time.Sleep(30 * time.Millisecond)
		}
	}
	for range 2 {
		s.Draw(&screen)
		time.Sleep(time.Second / 3)
		s.Draw(&emptyscreen)
		time.Sleep(time.Second / 3)
	}
	for i := range screen {
		for j := range screen[i] {
			screen[i][j] = 0xffff_ffff
		}
	}
	s.Draw(&screen)
	time.Sleep(2 * time.Second)
}

func testScreenBus(s *HUB40Screen) {
	var screen [4][4]uint32
	buf := unsafe.Slice(&screen[0][0], 4*4)
	for j := range buf {
		for i := 0; i < 24; i++ {
			mask := uint32(1) << i
			buf[j] = mask
			buf[0] |= 1
			buf[len(buf)-1] |= 1 << 23
			s.Draw(&screen)
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// HUB40Screen is a RGB single-bit-per-pixel screen that works on the basis of
// several shift registers arranges in two rows. Each clock represents 6 bits
// over the parallel-6 bus, so two LEDs per clock. The device I am using has
// no markings other than "HUB40" and "OF-20R3A2-0816-V21JC" which is likely not a real model.
// It seems to be a one-off design for a media company. Data lines are:
//   - RD0..BD1: Six Red/Green/Blue data lines for row 0 and row 1.
//   - OE: Output enable. Turns entire display on or off. Is negated.
//   - LAT: Latch. Latches shift registers values. Basically switches display buffer to the one written most recently.
type HUB40Screen struct {
	// data pins.
	lat pinout
	oe  pinout
	p6  Parallel6Bus
}

func (s *HUB40Screen) Latch() {
	s.lat.High()
	runtime.Gosched()
	s.lat.Low()
}

func (s *HUB40Screen) Enable(b bool) {
	s.oe(!b)
}

// Write writes the screen data without latching.
func (s *HUB40Screen) Write(screen *[4][4]uint32) (err error) {
	buf := unsafe.Slice(&screen[0][0], 4*4)
	return s.p6.Tx24(buf)
}

// Draw writes and then latches screen data.
func (s *HUB40Screen) Draw(screen *[4][4]uint32) (err error) {
	err = s.Write(screen)
	if err == nil {
		s.Latch()
	}
	return err
}

type bitbangParallel6 struct {
	rd0, gd0, bd0, rd1, gd1, bd1 pinout
	clk                          pinout
}

func (s *bitbangParallel6) Tx24(buf []uint32) error {
	for i := range buf {
		s.clkout24(buf[i])
	}
	return nil
}

func (s *bitbangParallel6) clkout24(v uint32) {
	const bits6 = 0b11_1111
	s.rawclkout6(uint8(v & bits6))
	s.rawclkout6(uint8((v >> 6) & bits6))
	s.rawclkout6(uint8((v >> 12) & bits6))
	s.rawclkout6(uint8((v >> 18) & bits6))
}

func (s *bitbangParallel6) rawclkout6(rgbBits uint8) {
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
