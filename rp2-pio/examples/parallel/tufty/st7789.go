package main

import (
	"errors"
	"image/color"
	"machine"
	"time"

	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// ST7789 wraps a Parallel ST7789 Display
type ST7789 struct {
	// Pins
	cs machine.Pin
	dc machine.Pin
	rd machine.Pin
	bl machine.Pin

	pl *piolib.Parallel

	// General Display Stuff
	width    uint16
	height   uint16
	rotation Rotation

	//Copied stuff from the TinyGo Drivers implementation
	buf [6]byte
}

func (st *ST7789) SetBacklight(on bool) {
	if st.bl == machine.NoPin {
		return
	}
	pwm := machine.PWM1 // LCD LED on Tufty2040 corresponds to PWM1.
	pwm.Configure(machine.PWMConfig{})
	ch, err := pwm.Channel(st.bl)
	if err != nil {
		return
	}
	if on {
		pwm.Set(ch, pwm.Top()) // full brightness
		return
	}
	pwm.Set(ch, 0) // off
}

func (st *ST7789) CommonInit() {
	st.dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	st.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	st.cs.High() // Deassert chip select until the first command.

	// Keep the panel dark until the init sequence has finished.
	st.SetBacklight(false)

	st.command(SWRESET, []byte{}) // software reset

	time.Sleep(150 * time.Millisecond) // reset needs 120ms before further commands

	st.command(COLMOD, []byte{0x05})                          // 16 bits per pixel
	st.command(PORCTRL, []byte{0x0c, 0x0c, 0x00, 0x33, 0x33}) // porch intervals
	st.command(LCMCTRL, []byte{0x2c})                         // LCM control
	st.command(VDVVRHEN, []byte{0x01})                        // take VDV and VRH from the command registers
	st.command(VRHS, []byte{0x12})                            // VRH ~4.45V
	st.command(VDVS, []byte{0x20})                            // VDV 0V
	st.command(PWCTRL1, []byte{0xa4, 0xa1})                   // AVDD 6.8V, AVCL -4.8V, VDS 2.3V
	st.command(FRCTRL2, []byte{0x0f})                         // 60Hz frame rate
	st.command(RAMCTRL, []byte{0x00, 0xc0})                   // MCU interface, big endian pixel data
	st.command(GCTRL, []byte{0x35})                           // gate voltages VGH 13.26V, VGL -10.43V
	st.command(VCOMS, []byte{0x1b})                           // VCOM 0.875V

	// Gamma correction curves tuned for the Tufty panel.
	st.command(GMCTRP1, []byte{0xf0, 0x00, 0x06, 0x04, 0x05, 0x05, 0x31, 0x44, 0x48, 0x36, 0x12, 0x12, 0x2b, 0x34}) // positive
	st.command(GMCTRN1, []byte{0xf0, 0x0b, 0x0f, 0x0f, 0x0d, 0x26, 0x31, 0x43, 0x47, 0x38, 0x14, 0x14, 0x2c, 0x32}) // negative

	st.command(INVON, []byte{})  // set inversion mode
	st.command(SLPOUT, []byte{}) // leave sleep mode

	time.Sleep(100 * time.Millisecond) // sleep out needs 120ms before the display is driven

	st.configureDisplayRotation(st.rotation) // set the addressing window and scan order

	st.command(TEON, []byte{0x00})      // enable frame sync signal
	st.command(STE, []byte{0x00, 0x00}) // sync on scanline 0
	st.command(DISPON, []byte{})        // turn display on

	// Panel is now driven, so it is safe to light the backlight.
	if st.bl != machine.NoPin {
		time.Sleep(50 * time.Millisecond)
		st.SetBacklight(true)
	}
}

func (st *ST7789) configureDisplayRotation(rotation Rotation) {
	var madctl uint8
	var rotate180 bool
	caset := []uint16{0, 0}
	raset := []uint16{0, 0}

	if rotation == Rotation180 || rotation == Rotation90 {
		rotate180 = true
	}
	if rotation == Rotation90 || rotation == Rotation270 {
		st.width, st.height = st.height, st.width
	}

	caset[0] = 0
	caset[1] = 319
	raset[0] = 0
	raset[1] = 239
	if rotate180 {
		madctl = ROW_ORDER
	} else {
		madctl = COL_ORDER
	}
	madctl |= SWAP_XY | SCAN_ORDER

	// CASET/RASET take big-endian 16 bit values.
	st.command(CASET, []byte{byte(caset[0] >> 8), byte(caset[0]), byte(caset[1] >> 8), byte(caset[1])})
	st.command(RASET, []byte{byte(raset[0] >> 8), byte(raset[0]), byte(raset[1] >> 8), byte(raset[1])})
	st.command(MADCTL, []byte{madctl})
}

func (st *ST7789) command(command byte, data []byte) {
	st.dc.Low()
	st.cs.Low()
	st.pl.Tx8([]byte{command})

	if len(data) > 0 {
		st.dc.High()
		st.pl.Tx8(data)
	}
	// Tx8 returns on TX-stall, which can be a couple of PIO cycles before the
	// final WR rising edge. Let the bus settle before deasserting CS.
	time.Sleep(10 * time.Microsecond)
	st.cs.High()
}

func RGBATo565(c color.RGBA) uint16 {
	r, g, b, _ := c.RGBA()
	return uint16((r & 0xF800) +
		((g & 0xFC00) >> 5) +
		((b & 0xF800) >> 11))
}

func (st *ST7789) Size() (w, h int16) {
	return int16(st.width), int16(st.height)
}

func (st *ST7789) setWindow(x, y, w, h int16) {
	copy(st.buf[:4], []uint8{uint8(x >> 8), uint8(x), uint8((x + w - 1) >> 8), uint8(x + w - 1)})
	st.command(CASET, st.buf[:4])
	copy(st.buf[:4], []uint8{uint8(y >> 8), uint8(y), uint8((y + h - 1) >> 8), uint8(y + h - 1)})
	st.command(RASET, st.buf[:4])
}

func (st *ST7789) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	k, i := st.Size()
	if x < 0 || y < 0 || width <= 0 || height <= 0 ||
		x >= k || (x+width) > k || y >= i || (y+height) > i {
		return errors.New("rectangle coordinates outside display area")
	}
	st.setWindow(x, y, width, height)

	c565 := RGBATo565(c)
	// Pre-fill a small chunk once and stream it repeatedly rather than
	// allocating a whole framebuffer.
	var chunk [512]byte
	for j := 0; j < len(chunk); j += 2 {
		chunk[j] = uint8(c565 >> 8)
		chunk[j+1] = uint8(c565)
	}

	st.dc.Low()
	st.cs.Low()
	st.pl.Tx8([]byte{RAMWR})
	st.dc.High()
	remaining := int(width) * int(height) * 2
	for remaining > 0 {
		n := remaining
		if n > len(chunk) {
			n = len(chunk)
		}
		if err := st.pl.Tx8(chunk[:n]); err != nil {
			st.cs.High()
			return err
		}
		remaining -= n
	}
	time.Sleep(10 * time.Microsecond)
	st.cs.High()
	return nil
}
