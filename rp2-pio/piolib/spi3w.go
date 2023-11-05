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
	dma    dmaChannel
}

func NewSPI3w(sm pio.StateMachine, dio, clk machine.Pin, baud uint32) (*SPI3w, error) {
	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err // Early return on back clock.
	}

	// https://github.com/embassy-rs/embassy/blob/c4a8b79dbc927e46fcc71879673ad3410aa3174b/cyw43-pio/src/lib.rs#L90
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

func (spi SPI3w) CmdWrite(cmd uint32, w []uint32) error {
	writeBits := len(w)*32 - 1
	const readBits = 31

	spi.prepTx(uint32(readBits), uint32(writeBits))

	spi.sm.TxPut(cmd)
	if spi.dma.IsValid() {
		return spi.writeDMA(w)
	}
	return spi.write(w)
}

func (spi *SPI3w) CmdRead(cmd uint32, r []uint32) error {
	const writeBits = 31
	readBits := len(r)*32 + 32 - 1
	spi.prepTx(uint32(readBits), uint32(writeBits))

	spi.sm.TxPut(cmd)
	if spi.dma.IsValid() {
		return spi.readDMA(r)
	}
	return spi.read(r)
}

func (spi *SPI3w) prepTx(readbits, writebits uint32) {
	spi.sm.SetEnabled(false)

	spi.sm.ClearFIFOs()
	spi.sm.SetX(readbits)
	spi.sm.SetY(writebits)
	spi.sm.Exec(pio.EncodeSet(pio.SrcDestPinDirs, 1)) // Set Pindir out.
	spi.sm.Jmp(spi.offset+spi3wWrapTarget, pio.JmpAlways)

	spi.sm.SetEnabled(true)
}

func (spi *SPI3w) EnableDMA(enabled bool) error {
	if !enabled {
		if spi.dma.IsValid() {
			spi.dma.Unclaim()
		}
		return nil
	}
	channel, ok := _DMA.ClaimChannel()
	if !ok {
		return errDMAUnavail
	}
	spi.dma = channel
	return nil
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

func (spi *SPI3w) readDMA(r []uint32) error {
	dreq := dmaPIO_RxDREQ(spi.sm)
	spi.dma.Pull32(r, &spi.sm.RxReg().Reg, dreq)
	return nil
}

func (spi *SPI3w) write(w []uint32) error {
	i := 0
	retries := timeoutRetries
	for i < len(w) && retries > 0 {
		if spi.sm.IsTxFIFOFull() {
			gosched()
			retries--
			continue
		}
		spi.sm.TxPut(w[i])
		i++
	}
	if retries <= 0 {
		return errTimeout
	}
	return nil
}

func (spi *SPI3w) writeDMA(w []uint32) error {
	dreq := dmaPIO_TxDREQ(spi.sm)
	spi.dma.Push32(&spi.sm.TxReg().Reg, w, dreq)
	return nil
}
