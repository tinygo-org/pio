package main

import (
	"image/color"
	"machine"
	"math"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Pimoroni Tufty definitions https://tinygo.org/docs/reference/microcontrollers/tufty2040/
const (
	csPin  = machine.GPIO10 // LCD_CS
	dcPin  = machine.GPIO11 // LCD_DC
	wrPin  = machine.GPIO12 // LCD_WR
	db0Pin = machine.GPIO14 // LCD_DB0..DB7 = GPIO14..GPIO21
	rdPin  = machine.GPIO13 // LCD_RD
	blPin  = machine.GPIO2  // LCD_BACKLIGHT
)

// busBaud controls the PIO parallel bus clock rate driving WR.
//
// The parallel PIO program in piolib is three instructions long, so the PIO
// state machine clock runs at 3 * busBaud. The ST7789 8080-II parallel
// interface specifies a minimum write cycle of 66 ns (~15.15 MHz), and
// 15 MHz has been verified visually clean on a Tufty 2040 panel — we run
// right at the datasheet ceiling because the bouncing-rect demo is
// bus-limited and the extra ~9% throughput is worth having. Measured on
// hardware: ~37 FPS @ 12.5 MHz, ~42 FPS @ 15 MHz with a full 320x240x16bpp
// framebuffer transfer every frame.
const busBaud = 15_000_000

// Compile-time assertion that ST7789 satisfies our local Displayer contract.
// The signatures match tinygo.org/x/drivers.Displayer byte-for-byte so
// downstream code can substitute that interface without changes here.
var _ Displayer = (*ST7789)(nil)

// framebuffer is a fixed-size RGB565 buffer sized for the panel and placed in
// .bss so the runtime doesn't have to satisfy a 154KB make() at startup.
const displayW, displayH = 320, 240

var framebuffer [displayW * displayH * 2]byte

var display ST7789

func main() {
	// Configure control pins to safe idle levels BEFORE bringing up the PIO
	// parallel bus. If CS or DC are floating while the PIO state machine
	// starts and puts its initial (zeroed) OSR contents on the bus, the panel
	// intermittently latches stray bytes as commands, leaving the display in
	// an unknown state that manifests as "sometimes it doesn't come up".
	csPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	csPin.High() // CS idle high (panel deselected)
	dcPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dcPin.High() // DC idle in data mode
	rdPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	rdPin.High() // RD held high so the panel accepts writes

	sm, _ := pio.PIO0.ClaimStateMachine()

	// Drive the 8 bit parallel bus from PIO, clocking data out on WR.
	p8tx, err := piolib.NewParallel(sm, piolib.ParallelConfig{
		Baud:        busBaud,
		Clock:       wrPin,
		DataBase:    db0Pin,
		BusWidth:    8,
		BitsPerPull: 8,
	})
	if err != nil {
		panic(err.Error())
	}
	display = ST7789{
		pl:       p8tx,
		cs:       csPin,
		dc:       dcPin,
		rd:       rdPin,
		bl:       blPin,
		width:    displayW,
		height:   displayH,
		rotation: Rotation0,
		fb:       framebuffer[:],
	}

	// Feed the PIO TX FIFO by DMA so large pixel writes do not block on the CPU.
	if err := display.pl.EnableDMA(true); err != nil {
		panic(err.Error())
	}

	display.CommonInit()

	// Cycle through three demos at each of the four rotations. Each demo
	// draws into the shared framebuffer with SetPixel and pushes it with
	// Display(), so the Displayer interface gets a real workout in every
	// orientation and the mapping foundation from PR #55 is exercised.
	const demoDuration = 6 * time.Second
	rotations := []Rotation{Rotation0, Rotation90, Rotation180, Rotation270}
	for {
		for _, r := range rotations {
			display.configureDisplayRotation(r)
			runBouncingRects(&display, demoDuration)
			runMandelbrot(&display, demoDuration)
			runPlasma(&display, demoDuration)
		}
	}
}

// fillFB paints the whole framebuffer to a single RGB565 colour.
func fillFB(st *ST7789, c565 uint16) {
	hi := uint8(c565 >> 8)
	lo := uint8(c565)
	for i := 0; i < len(st.fb); i += 2 {
		st.fb[i] = hi
		st.fb[i+1] = lo
	}
}

// runBouncingRects animates a handful of solid rectangles ricocheting off
// the screen edges. Cheap to draw, and clearly shows that Display() is
// being called every frame.
func runBouncingRects(st *ST7789, d time.Duration) {
	w, h := st.Size()
	type rect struct {
		x, y, dx, dy, w, h int16
		c                  color.RGBA
	}
	rects := []rect{
		{20, 20, 3, 2, 40, 30, color.RGBA{255, 64, 64, 255}},
		{90, 60, -2, 3, 50, 20, color.RGBA{64, 255, 64, 255}},
		{160, 120, 4, -3, 30, 60, color.RGBA{64, 128, 255, 255}},
		{200, 40, -3, -2, 60, 40, color.RGBA{255, 255, 64, 255}},
	}
	bg := RGBATo565(color.RGBA{0, 0, 0, 255})
	deadline := time.Now().Add(d)
	frames := 0
	start := time.Now()
	for time.Now().Before(deadline) {
		fillFB(st, bg)
		for i := range rects {
			r := &rects[i]
			r.x += r.dx
			r.y += r.dy
			if r.x < 0 {
				r.x = 0
				r.dx = -r.dx
			}
			if r.y < 0 {
				r.y = 0
				r.dy = -r.dy
			}
			if r.x+r.w >= w {
				r.x = w - r.w - 1
				r.dx = -r.dx
			}
			if r.y+r.h >= h {
				r.y = h - r.h - 1
				r.dy = -r.dy
			}
			for py := r.y; py < r.y+r.h; py++ {
				for px := r.x; px < r.x+r.w; px++ {
					st.SetPixel(px, py, r.c)
				}
			}
		}
		if err := st.Display(); err != nil {
			println("Display:", err.Error())
			return
		}
		frames++
	}
	reportFPS("bouncing", frames, time.Since(start))
}

// runMandelbrot renders successive Mandelbrot frames zooming in slowly
// toward an interesting point. Uses Q6.26 fixed-point arithmetic so it
// runs at usable frame rates on the RP2040 (no hardware FPU).
func runMandelbrot(st *ST7789, d time.Duration) {
	w, h := st.Size()
	const (
		maxIter  = 24
		fracBits = 26             // Q6.26
		one      = int64(1) << 26 // 1.0
		four     = int64(4) << 26 // escape radius^2
	)
	// Target point (approx -0.7436, 0.1318) in Q6.26.
	targetRe := int64(math.Round(-0.743643887037151 * float64(one)))
	targetIm := int64(math.Round(0.131825904205330 * float64(one)))
	zoom := int64(3 * one) // horizontal span in fixed point
	deadline := time.Now().Add(d)
	frames := 0
	start := time.Now()
	for time.Now().Before(deadline) {
		// span-per-pixel = zoom / w
		stepX := zoom / int64(w)
		stepY := zoom / int64(w) // square pixels
		originRe := targetRe - stepX*int64(w)/2
		originIm := targetIm - stepY*int64(h)/2
		for py := int16(0); py < h; py++ {
			ci := originIm + stepY*int64(py)
			for px := int16(0); px < w; px++ {
				cr := originRe + stepX*int64(px)
				var zr, zi int64
				var it int
				for it = 0; it < maxIter; it++ {
					zr2 := (zr * zr) >> fracBits
					zi2 := (zi * zi) >> fracBits
					if zr2+zi2 > four {
						break
					}
					newZr := zr2 - zi2 + cr
					zi = ((zr*zi)>>fracBits)*2 + ci
					zr = newZr
				}
				st.SetPixel(px, py, iterColour(it, maxIter))
			}
		}
		if err := st.Display(); err != nil {
			println("Display:", err.Error())
			return
		}
		frames++
		// Zoom in ~15% per frame; wrap around when the field collapses.
		zoom = zoom * 85 / 100
		if zoom < one/1000 {
			zoom = 3 * one
		}
	}
	reportFPS("mandelbrot", frames, time.Since(start))
}

// iterColour maps a Mandelbrot iteration count to a smooth RGB565 palette
// via a small integer-only palette table.
func iterColour(it, maxIter int) color.RGBA {
	if it >= maxIter {
		return color.RGBA{0, 0, 0, 255}
	}
	// Simple hot/cold-ish palette entirely in integer math.
	t := (it * 255) / maxIter
	r := uint8(t)
	g := uint8((t * t) >> 8)
	b := uint8(255 - t)
	return color.RGBA{r, g, b, 255}
}

// sinTable holds 256 samples of sin(2*pi*i/256) scaled to int16.
var sinTable [256]int16

func init() {
	for i := 0; i < 256; i++ {
		sinTable[i] = int16(math.Round(math.Sin(2*math.Pi*float64(i)/256) * 127))
	}
}

// isin returns sin scaled to int16 (-127..127) for the Q0.8 angle a.
func isin(a int) int16 {
	return sinTable[uint8(a)]
}

// runPlasma renders an animated sinusoidal plasma effect using a sin LUT
// so it hits a real frame rate on the RP2040.
func runPlasma(st *ST7789, d time.Duration) {
	w, h := st.Size()
	deadline := time.Now().Add(d)
	frames := 0
	start := time.Now()
	t := 0
	for time.Now().Before(deadline) {
		for py := int16(0); py < h; py++ {
			for px := int16(0); px < w; px++ {
				// Combine four cheap sinusoids sampled from the LUT.
				v := int(isin(int(px)*4+t)) +
					int(isin(int(py)*5-t)) +
					int(isin(int(px+py)*3+t)) +
					int(isin(int(px-py)*2-t))
				// v is in ~[-508,508]; fold to 0..255.
				u := uint8(((v + 512) >> 2) & 0xff)
				r := uint8(isin(int(u))) + 128
				g := uint8(isin(int(u)+85)) + 128
				b := uint8(isin(int(u)+170)) + 128
				st.SetPixel(px, py, color.RGBA{r, g, b, 255})
			}
		}
		if err := st.Display(); err != nil {
			println("Display:", err.Error())
			return
		}
		frames++
		t += 3
	}
	reportFPS("plasma", frames, time.Since(start))
}

// reportFPS prints a one-line FPS summary for a demo run over UART.
func reportFPS(name string, frames int, elapsed time.Duration) {
	if frames == 0 || elapsed <= 0 {
		return
	}
	fps := float64(frames) * float64(time.Second) / float64(elapsed)
	println(name, "frames=", frames, "elapsed_ms=", int(elapsed/time.Millisecond), "fps=", int(fps*10), "/10")
}

type Displayer interface {
	// Size returns the current size of the display.
	Size() (x, y int16)

	// SetPixel modifies the internal buffer.
	SetPixel(x, y int16, c color.RGBA)

	// Display sends the buffer (if any) to the screen.
	Display() error
}

// Rotation is how much a display has been rotated. Displays can be rotated, and
// sometimes also mirrored.
type Rotation uint8

// Clockwise rotation of the screen.
const (
	Rotation0 = iota
	Rotation90
	Rotation180
	Rotation270
	Rotation0Mirror
	Rotation90Mirror
	Rotation180Mirror
	Rotation270Mirror
)
