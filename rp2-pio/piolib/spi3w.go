//go:build rp2040

package piolib

import (
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// SPI3 is a 3-wire SPI implementation for specialized use cases, such as
// the Pico W's on-board CYW43439 WiFi module. It uses a shared data input/output pin.
type SPI3w struct {
	sm     pio.StateMachine
	offset uint8
}

func NewSPI3(sm pio.StateMachine, dio, clk machine.Pin, baud uint32) (*SPI3w, error) {
	sm.Claim() // SM should be claimed beforehand, we just guarantee it's claimed.

	Pio := sm.PIO()
	offset, err := Pio.AddProgram(spi3wInstructions, spi3wOrigin)
	if err != nil {
		return nil, err
	}

	// Configure state machine.
	cfg := spi3wProgramDefaultConfig(offset)
	cfg.SetOutPins(dio, 1)
	cfg.SetSetPins(dio, 1)
	cfg.SetInPins(dio)
	cfg.SetSidesetPins(clk)
	cfg.SetOutShift(false, true, 32)
	cfg.SetInShift(false, true, 32)

	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	cfg.SetClkDivIntFrac(whole, frac)

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	dio.Configure(pinCfg)
	clk.Configure(pinCfg)
	Pio.HW().INPUT_SYNC_BYPASS.SetBits(1 << dio)

	// Initialize state machine.
	sm.Init(offset, cfg)
	pinMask := uint32(1<<dio | 1<<clk)
	sm.SetPindirsMasked(0, pinMask)
	sm.SetPinsMasked(0, pinMask)

	sm.SetEnabled(true)

	spiw := &SPI3w{
		sm:     sm,
		offset: offset,
	}
	return spiw, nil
}

func (spi *SPI3w) CmdRead(cmd uint32, r []uint32) error {
	spi.sm.SetEnabled(false)
	const writeBits = 31
	readBits := len(r)*32 + 32 - 1

	spi.sm.SetX(uint32(readBits))
	spi.sm.SetY(uint32(writeBits))
	spi.sm.Exec(pio.EncodeSet(pio.SrcDestPinDirs, 1)) // Set Pindir out.
	spi.sm.Jmp(spi.offset+spi3wWrapTarget, pio.JmpAlways)

	spi.sm.SetEnabled(true)

	spi.sm.TxPut(cmd)

	return spi.read(r)
}

func (spi *SPI3w) read(r []uint32) error {
	i := 0
	retries := timeoutRetries
	for i < len(r) && retries > 0 {
		if spi.sm.IsRxFIFOEmpty() {
			gosched()
			retries--
			continue
		}
		r[i] = spi.sm.RxGet()
		spi.sm.TxPut(r[i])
		i++
	}
	if retries <= 0 {
		return errTimeout
	}
	return nil
}
