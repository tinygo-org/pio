//go:build rp2040 || rp2350

package piolib

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// ST7789 implements a driver for ST7789 LCD displays using the 8080 parallel interface.
// The ST7789 is a common LCD controller used in many displays including the Tufty 2040.
//
// The 8080 parallel interface uses:
// - 8 data lines (D0-D7) for transferring pixel/command data
// - WRX (write clock) pin - driven by PIO sideset
// - RDX (read clock) pin - typically held high for write-only operation
// - DCX (data/command select) - driven by software
// - CS (chip select) - driven by software
//
// PIO Program Timing (meets ST7789 V_m constraint):
// The ST7789 requires data to be stable before the write clock rising edge (V_m parameter).
// The PIO program ensures this by:
// 1. Outputting data with clock LOW (side 0)
// 2. Raising clock HIGH (side 1) - display captures data at this point
// 3. Lowering clock again (side 0) - ready for next byte
type ST7789 struct {
	bus  *Parallel
	cs   machine.Pin
	dc   machine.Pin
	rd   machine.Pin
	bl   machine.Pin

	width  uint16
	height uint16
}

// ST7789Config configures the ST7789 display driver.
type ST7789Config struct {
	// Width and Height of the display in pixels.
	Width, Height uint16
	// Chip select pin (CSX).
	CS machine.Pin
	// Data/command select pin (DCX).
	DC machine.Pin
	// Read clock pin (RDX). Set to machine.NoPin if not used.
	// Should be held HIGH for write-only operation.
	RD machine.Pin
	// Backlight pin. Set to machine.NoPin if not used.
	BL machine.Pin
}

// NewST7789 creates a new ST7789 driver using the provided parallel bus.
// The parallel bus should be configured with:
// - BusWidth: 8
// - BitsPerPull: 8 (or multiples for DMA efficiency)
// - ShiftLeft: true for correct bit-to-pin mapping
// - FastMode: true for higher throughput (optional)
func NewST7789(sm pio.StateMachine, parallelCfg ParallelConfig, displayCfg ST7789Config) (*ST7789, error) {
	// Configure parallel bus for ST7789
	bus, err := NewParallel(sm, parallelCfg)
	if err != nil {
		return nil, err
	}

	display := &ST7789{
		bus:    bus,
		cs:     displayCfg.CS,
		dc:     displayCfg.DC,
		rd:     displayCfg.RD,
		bl:     displayCfg.BL,
		width:  displayCfg.Width,
		height: displayCfg.Height,
	}

	// Configure control pins
	display.dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	display.cs.Configure(machine.PinConfig{Mode: machine.PinOutput})
	display.cs.High() // Start with CS high (not selected)

	if display.rd != machine.NoPin {
		display.rd.Configure(machine.PinConfig{Mode: machine.PinOutput})
		display.rd.High() // Hold RD high for write-only operation
	}

	if display.bl != machine.NoPin {
		display.bl.Configure(machine.PinConfig{Mode: machine.PinOutput})
		display.bl.Low() // Start with backlight off
	}

	return display, nil
}

// EnableBacklight turns the backlight on or off.
func (st *ST7789) EnableBacklight(on bool) {
	if st.bl != machine.NoPin {
		if on {
			st.bl.High()
		} else {
			st.bl.Low()
		}
	}
}

// WriteCommand sends a command byte to the ST7789.
func (st *ST7789) WriteCommand(cmd byte) {
	st.cs.Low()
	st.dc.Low() // Command mode
	st.bus.Tx8([]byte{cmd})
	st.cs.High()
}

// WriteData sends data bytes to the ST7789.
func (st *ST7789) WriteData(data []byte) {
	st.cs.Low()
	st.dc.High() // Data mode
	st.bus.Tx8(data)
	st.cs.High()
}

// WriteCommandData sends a command followed by data bytes.
// This is the most common pattern for ST7789 commands.
func (st *ST7789) WriteCommandData(cmd byte, data []byte) {
	st.cs.Low()
	st.dc.Low() // Command mode
	st.bus.Tx8([]byte{cmd})
	if len(data) > 0 {
		st.dc.High() // Data mode
		st.bus.Tx8(data)
	}
	st.cs.High()
}

// Initialize performs the standard ST7789 initialization sequence.
// This includes reset, power control, gamma settings, and display enable.
// Different displays may require different initialization sequences;
// this provides a common baseline that works for many ST7789 displays.
func (st *ST7789) Initialize() {
	// Software reset
	st.WriteCommand(CMD_SWRESET)
	time.Sleep(150 * time.Millisecond)

	// Exit sleep mode
	st.WriteCommand(CMD_SLPOUT)
	time.Sleep(10 * time.Millisecond)

	// Set color mode to 16-bit (RGB565)
	st.WriteCommandData(CMD_COLMOD, []byte{0x05})

	// Memory access control (MADCTL)
	// Configure for correct display orientation
	st.WriteCommandData(CMD_MADCTL, []byte{0x00})

	// Porch control
	st.WriteCommandData(CMD_PORCTRL, []byte{0x0C, 0x0C, 0x00, 0x33, 0x33})

	// LCM control
	st.WriteCommandData(CMD_LCMCTRL, []byte{0x2C})

	// VDV and VRH enable
	st.WriteCommandData(CMD_VDVVRHEN, []byte{0x01})

	// VRH set
	st.WriteCommandData(CMD_VRHS, []byte{0x12})

	// VDV set
	st.WriteCommandData(CMD_VDVS, []byte{0x20})

	// Power control 1
	st.WriteCommandData(CMD_PWCTRL1, []byte{0xA4, 0xA1})

	// Frame rate control
	st.WriteCommandData(CMD_FRCTRL2, []byte{0x0F})

	// Gate control
	st.WriteCommandData(CMD_GCTRL, []byte{0x35})

	// VCOM setting
	st.WriteCommandData(CMD_VCOMS, []byte{0x1F})

	// Positive gamma correction
	st.WriteCommandData(CMD_GMCTRP1, []byte{
		0xD0, 0x08, 0x11, 0x08, 0x0C, 0x15, 0x39, 0x33,
		0x50, 0x36, 0x13, 0x14, 0x29, 0x2D,
	})

	// Negative gamma correction
	st.WriteCommandData(CMD_GMCTRN1, []byte{
		0xD0, 0x08, 0x10, 0x08, 0x06, 0x06, 0x39, 0x44,
		0x51, 0x0B, 0x16, 0x14, 0x2F, 0x31,
	})

	// Inversion on (for proper color display)
	st.WriteCommand(CMD_INVON)

	// Display on
	st.WriteCommand(CMD_DISPON)
	time.Sleep(100 * time.Millisecond)

	// Enable backlight
	st.EnableBacklight(true)
}

// SetWindow sets the drawing window for subsequent pixel data.
func (st *ST7789) SetWindow(x0, y0, x1, y1 uint16) {
	// Column address set (CASET)
	st.WriteCommandData(CMD_CASET, []byte{
		byte(x0 >> 8), byte(x0),
		byte(x1 >> 8), byte(x1),
	})
	// Row address set (RASET)
	st.WriteCommandData(CMD_RASET, []byte{
		byte(y0 >> 8), byte(y0),
		byte(y1 >> 8), byte(y1),
	})
	// Write to memory
	st.WriteCommand(CMD_RAMWR)
}

// FillScreen fills the entire screen with a single color (RGB565).
func (st *ST7789) FillScreen(color uint16) {
	st.SetWindow(0, 0, st.width-1, st.height-1)

	// Prepare pixel buffer (2 bytes per pixel)
	highByte := byte(color >> 8)
	lowByte := byte(color & 0xFF)

	// Create a buffer for efficient DMA transfer
	// Using chunks of 512 pixels (1024 bytes) for efficiency
	const chunkPixels = 512
	buf := make([]byte, chunkPixels*2)
	for i := 0; i < chunkPixels; i++ {
		buf[i*2] = highByte
		buf[i*2+1] = lowByte
	}

	// CS must stay low for entire fill operation
	st.cs.Low()
	st.dc.High() // Data mode (after RAMWR command)

	totalPixels := uint32(st.width) * uint32(st.height)
	pixelsSent := uint32(0)

	for pixelsSent < totalPixels {
		pixelsToSend := chunkPixels
		if totalPixels - pixelsSent < chunkPixels {
			pixelsToSend = totalPixels - pixelsSent
			// Adjust buffer for last chunk
			buf = buf[:pixelsToSend*2]
		}
		st.bus.Tx8(buf)
		pixelsSent += pixelsToSend
	}

	st.cs.High()
}

// FillRectangle fills a rectangle with a single color (RGB565).
func (st *ST7789) FillRectangle(x, y, width, height uint16, color uint16) {
	st.SetWindow(x, y, x+width-1, y+height-1)

	highByte := byte(color >> 8)
	lowByte := byte(color & 0xFF)

	const chunkPixels = 512
	buf := make([]byte, chunkPixels*2)
	for i := 0; i < chunkPixels; i++ {
		buf[i*2] = highByte
		buf[i*2+1] = lowByte
	}

	st.cs.Low()
	st.dc.High()

	totalPixels := uint32(width) * uint32(height)
	pixelsSent := uint32(0)

	for pixelsSent < totalPixels {
		pixelsToSend := chunkPixels
		if totalPixels - pixelsSent < chunkPixels {
			pixelsToSend = totalPixels - pixelsSent
			buf = buf[:pixelsToSend*2]
		}
		st.bus.Tx8(buf)
		pixelsSent += pixelsToSend
	}

	st.cs.High()
}

// Size returns the display dimensions.
func (st *ST7789) Size() (uint16, uint16) {
	return st.width, st.height
}

// ST7789 command constants
const (
	CMD_SWRESET   byte = 0x01 // Software reset
	CMD_SLPOUT    byte = 0x11 // Sleep out
	CMD_INVON     byte = 0x21 // Inversion on
	CMD_DISPON    byte = 0x29 // Display on
	CMD_CASET     byte = 0x2A // Column address set
	CMD_RASET     byte = 0x2B // Row address set
	CMD_RAMWR     byte = 0x2C // Memory write
	CMD_MADCTL    byte = 0x36 // Memory access control
	CMD_COLMOD    byte = 0x3A // Color mode
	CMD_PORCTRL   byte = 0xB2 // Porch control
	CMD_GCTRL     byte = 0xB7 // Gate control
	CMD_VCOMS     byte = 0xBB // VCOM setting
	CMD_LCMCTRL   byte = 0xC0 // LCM control
	CMD_VDVVRHEN  byte = 0xC2 // VDV and VRH enable
	CMD_VRHS      byte = 0xC3 // VRH set
	CMD_VDVS      byte = 0xC4 // VDV set
	CMD_FRCTRL2   byte = 0xC6 // Frame rate control
	CMD_PWCTRL1   byte = 0xD0 // Power control 1
	CMD_GMCTRP1   byte = 0xE0 // Positive gamma
	CMD_GMCTRN1   byte = 0xE1 // Negative gamma

	// Additional commands for specific configurations
	CMD_RAMCTRL   byte = 0xB0 // RAM control (fixes horizontal banding)
	CMD_PWCTRL2   byte = 0xD1 // Power control 2
)

// MADCTL flags for orientation configuration
const (
	MADCTL_MY  byte = 0x80 // Row address order
	MADCTL_MX  byte = 0x40 // Column address order
	MADCTL_MV  byte = 0x20 // Row/Column exchange
	MADCTL_ML  byte = 0x10 // Scan address order
	MADCTL_RGB byte = 0x00 // RGB order
	MADCTL_BGR byte = 0x08 // BGR order
	MADCTL_MH  byte = 0x04 // Horizontal refresh order
)

// ConfigureRotation sets the display orientation.
func (st *ST7789) ConfigureRotation(rotation uint8) {
	var madctl byte
	switch rotation {
	case 0:
		madctl = MADCTL_MX | MADCTL_MV | MADCTL_BGR
	case 90:
		madctl = MADCTL_BGR
	case 180:
		madctl = MADCTL_MY | MADCTL_MV | MADCTL_BGR
	case 270:
		madctl = MADCTL_MX | MADCTL_MY | MADCTL_BGR
	default:
		madctl = MADCTL_MX | MADCTL_MV | MADCTL_BGR
	}
	st.WriteCommandData(CMD_MADCTL, []byte{madctl})
}

// ApplyRAMControlFix applies the RAMCTRL (0xB0) fix that eliminates
// horizontal banding on some ST7789 displays like the Tufty 2040.
// This is the critical fix mentioned in the PIO PR #37.
func (st *ST7789) ApplyRAMControlFix() {
	st.WriteCommandData(CMD_RAMCTRL, []byte{0x00, 0xC0})
}