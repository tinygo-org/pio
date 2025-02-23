//go:build rp2040 || rp2350

package piolib

import (
	"image/color"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// WS2812B is an RGB LED strip controller implementation, also known as NeoPixel.
type WS2812B struct {
	sm     pio.StateMachine
	dma    dmaChannel
	offset uint8
}

func NewWS2812B(sm pio.StateMachine, pin machine.Pin) (*WS2812B, error) {
	// https://cdn-shop.adafruit.com/datasheets/WS2812B.pdf
	const (
		baseline      = 1250.
		baselinesplit = baseline / 3
		cycle         = baselinesplit / 3
		freq          = uint32(1e9 / cycle)
	)
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	cpufreq := machine.CPUFrequency()
	// whole, frac, err := pio.ClkDivFromPeriod(period, cpufreq)
	whole, frac, err := pio.ClkDivFromFrequency(freq, cpufreq)
	if err != nil {
		return nil, err
	}
	// We add the program to PIO memory and store it's offset.
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812b_ledInstructions, ws2812b_ledOrigin)
	if err != nil {
		return nil, err
	}
	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)
	cfg := ws2812b_ledProgramDefaultConfig(offset)
	cfg.SetSetPins(pin, 1)
	// We only use Tx FIFO, so we set the join to Tx.
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	cfg.SetClkDivIntFrac(whole, frac)
	cfg.SetOutShift(false, true, 24)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	dev := &WS2812B{sm: sm, offset: offset}
	return dev, nil
}

// PutRGB puts a RGB color in the transmit queue. If Queue if full will be discarded.
func (ws *WS2812B) PutRGB(r, g, b uint8) {
	// Shift occurs to left for WS2812B to interpret correctly.
	color := uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
	ws.PutRaw(color)
}

// PutRaw puts a raw color value in the PIO state machine queue. The grb uint32 is a WS2812B color
// which can be created with 3 uint8 color values:
//
//	color := uint32(green)<<24 | uint32(red)<<16 | uint32(blue)<<8
func (ws *WS2812B) PutRaw(grb uint32) {
	ws.sm.TxPut(grb)
}

// IsQueueFull returns true if the PIO Tx FIFO has 8 elements and will discard next call to PutRaw, PutRGB or PutColor.
func (ws *WS2812B) IsQueueFull() bool {
	return ws.sm.IsTxFIFOFull()
}

// PutColor wraps PutRGB for a [color.Color] type.
func (ws *WS2812B) PutColor(c color.Color) {
	r16, g16, b16, _ := c.RGBA()
	ws.PutRGB(uint8(r16>>8), uint8(g16>>8), uint8(b16>>8))
}

// WriteRaw writes raw GRB values to a strip of WS2812B LEDs. Each uint32 is a WS2812B color
// which can be created with 3 uint8 color values::
//
//	color := uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
func (ws *WS2812B) WriteRaw(rawGRB []uint32) error {
	if ws.IsDMAEnabled() {
		return ws.writeDMA(rawGRB)
	}
	dl := ws.dma.dl.newDeadline()
	i := 0
	for i < len(rawGRB) {
		if ws.IsQueueFull() {
			if dl.expired() {
				return errTimeout
			}
			gosched()
			continue
		}
		ws.sm.TxPut(rawGRB[i])
		i++
	}
	return nil
}

// EnableDMA enables DMA for vectorized writes.
func (ws *WS2812B) EnableDMA(enabled bool) error {
	dmaAlreadyEnabled := ws.IsDMAEnabled()
	if !enabled || dmaAlreadyEnabled {
		if !enabled && dmaAlreadyEnabled {
			ws.dma.Unclaim()
			ws.dma = dmaChannel{} // Invalidate DMA channel.
		}
		return nil
	}
	channel, ok := _DMA.ClaimChannel()
	if !ok {
		return errDMAUnavail
	}
	channel.dl = ws.dma.dl // Copy deadline.
	ws.dma = channel
	return nil
}

func (ws *WS2812B) writeDMA(w []uint32) error {
	dreq := dmaPIO_TxDREQ(ws.sm)
	err := ws.dma.Push32(&ws.sm.TxReg().Reg, w, dreq)
	if err != nil {
		return err
	}
	return nil
}

// IsDMAEnabled returns true if DMA is enabled.
func (ws *WS2812B) IsDMAEnabled() bool {
	return ws.dma.IsValid()
}
