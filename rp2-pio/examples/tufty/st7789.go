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

	pl *piolib.Parallel8Tx

	// General Display Stuff
	width    uint16
	height   uint16
	rotation Rotation

	//Copied stuff from the TinyGo Drivers implementation
	buf [6]byte
}

func (st *ST7789) SetBacklight(on bool) {
	if st.bl != machine.NoPin {
		pwm := machine.PWM1 // LCD LED on Tufty2040 corresponds to PWM1.
		// Configure the PWM
		pwm.Configure(machine.PWMConfig{})
		ch, err := pwm.Channel(st.bl)
		if err != nil {
			println(err.Error())
			return
		}
		if on {
			pwm.Set(ch, pwm.Top())
			return
		}
		pwm.Set(ch, 0)
		return
	}
	println("no backlight pin defined")
}

func (st *ST7789) CommonInit() {
	st.dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	st.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Configure Backlight Pin
	st.SetBacklight(false)

	println("SWRESET")
	st.command(SWRESET, []byte{})

	time.Sleep(150 * time.Millisecond)

	//Common Init
	println("TEON")
	st.command(TEON, []byte{})
	println("COLMOD")
	st.command(COLMOD, []byte{0x05}) // 16 bits per pixel
	println("PORCTRL")
	st.command(PORCTRL, []byte{0x0c, 0x0c, 0x00, 0x33, 0x33})
	println("LCMCTRL")
	st.command(LCMCTRL, []byte{0x2c})
	println("VDVVHREN")
	st.command(VDVVRHEN, []byte{0x01})
	println("VRHS")
	st.command(VRHS, []byte{0x12})
	println("VDVS")
	st.command(VDVS, []byte{0x20})
	println("PWCTRL1")
	st.command(PWCTRL1, []byte{0xa4, 0xa1})
	println("FRCTRL2")
	st.command(FRCTRL2, []byte{0x0f})

	// Tufty is 320x240
	println("GCTRL")
	st.command(GCTRL, []byte{0x35})
	println("VCOMS")
	st.command(VCOMS, []byte{0x1f})
	println("0xD6")
	st.command(0xD6, []byte{0xa1}) // ???
	println("GMCTRP1")
	st.command(GMCTRP1, []byte{0xD0, 0x08, 0x11, 0x08, 0x0C, 0x15, 0x39, 0x33, 0x50, 0x36, 0x13, 0x14, 0x29, 0x2D})
	println("GMCTRN1")
	st.command(GMCTRN1, []byte{0xD0, 0x08, 0x10, 0x08, 0x06, 0x06, 0x39, 0x44, 0x51, 0x0B, 0x16, 0x14, 0x2F, 0x31})

	println("INVON")
	st.command(INVON, []byte{})
	println("SLPOUT")
	st.command(SLPOUT, []byte{})
	println("DISPON")
	st.command(DISPON, []byte{})

	time.Sleep(100 * time.Millisecond)

	// Configure Display Rotation
	st.configureDisplayRotation(st.rotation)

	println("Turning on backlight")
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

	caset[0] = (caset[0] << 8) | ((caset[0] >> 8) & 0xFF)
	caset[1] = (caset[1] << 8) | ((caset[1] >> 8) & 0xFF)
	raset[0] = (raset[0] << 8) | ((raset[0] >> 8) & 0xFF)
	raset[1] = (raset[1] << 8) | ((raset[1] >> 8) & 0xFF)

	st.command(CASET, []byte{byte(caset[0] >> 8), byte(caset[0] & 0xff), byte(caset[1] >> 8), byte(caset[1] & 0xff)})
	st.command(CASET, []byte{byte(raset[0] >> 8), byte(raset[0] & 0xff), byte(raset[1] >> 8), byte(raset[1] & 0xff)})
	st.command(MADCTL, []byte{madctl})
}

func (st *ST7789) command(command byte, data []byte) {
	st.dc.Low()
	st.cs.Low()
	st.pl.Write([]byte{command})
	// st.writeBlockingParallel([]byte{command}, 1)

	if len(data) > 0 {
		st.dc.High()
		st.pl.Write(data)
		// st.writeBlockingParallel(data, len(data))
	}
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
	x += 0
	y += 0
	copy(st.buf[:4], []uint8{uint8(x >> 8), uint8(x), uint8((x + w - 1) >> 8), uint8(x + w - 1)})
	st.command(CASET, st.buf[:4])
	copy(st.buf[:4], []uint8{uint8(y >> 8), uint8(y), uint8((y + h - 1) >> 8), uint8(y + h - 1)})
	st.command(RASET, st.buf[:4])
	st.command(RAMWR, []byte{})
}

func (st *ST7789) FillRectangle(x, y, width, height int16, c color.RGBA) error {
	k, i := st.Size()
	if x < 0 || y < 0 || width <= 0 || height <= 0 ||
		x >= k || (x+width) > k || y >= i || (y+height) > i {
		return errors.New("rectangle coordinates outside display area")
	}
	st.setWindow(x, y, width, height)
	c565 := RGBATo565(c)
	c1 := uint8(c565 >> 8)
	c2 := uint8(c565)

	fb := make([]uint8, st.width*st.height*2)
	for i := 0; i < len(fb)/2; i++ {
		fb[i*2] = c1
		fb[i*2+1] = c2
	}
	st.command(RAMWR, fb)
	return nil
}
