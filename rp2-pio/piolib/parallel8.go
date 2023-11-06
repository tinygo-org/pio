//go:build rp2040

package piolib

import (
	"errors"
	"machine"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// Parallel8Tx is a 8-wire, only send Parallel implementation.
type Parallel8Tx struct {
	sm     pio.StateMachine
	offset uint8
	dma    dmaChannel
}

// unused for now.
const noDMA uint32 = 0xffff_ffff

func NewParallel8Tx(sm pio.StateMachine, wr, dStart machine.Pin, baud uint32) (*Parallel8Tx, error) {
	sm.Claim() // SM should be claimed beforehand, we just guarantee it's claimed.
	const nPins = 8
	if dStart+nPins > 31 {
		return nil, errors.New("invalid D0..D7 pin range")
	}
	baud *= 6 // ??? why 6?
	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(parallel8Instructions, parallel8Origin)
	if err != nil {
		return nil, err
	}

	// Configure pins.
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	for i := dStart; i < dStart+nPins; i++ {
		i.Configure(pinCfg)
	}
	wr.Configure(pinCfg)
	sm.SetPindirsConsecutive(wr, 1, true)
	sm.SetPindirsConsecutive(dStart, nPins, true)

	cfg := parallel8ProgramDefaultConfig(offset)

	cfg.SetOutPins(dStart, nPins)
	cfg.SetSidesetPins(wr)
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	cfg.SetOutShift(true, true, nPins)

	cfg.SetClkDivIntFrac(whole, frac)

	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	return &Parallel8Tx{sm: sm, offset: offset}, nil
}

func (pl *Parallel8Tx) Write(data []uint8) error {
	if pl.IsDMAEnabled() {
		return pl.dmaWrite(data)
	}
	retries := int8(127)
	for _, char := range data {
		if !pl.sm.IsTxFIFOFull() {
			pl.sm.TxPut(uint32(char))
		} else if retries > 0 {
			gosched()
			retries--
		} else {
			return errTimeout
		}
	}
	return nil
}

func (pl *Parallel8Tx) IsDMAEnabled() bool {
	return pl.dma.IsValid()
}

func (pl *Parallel8Tx) EnableDMA(enabled bool) error {
	if !pl.sm.IsValid() {
		return errors.New("PIO Statemachine needs initializing") //Not initialized
	}
	dmaAlreadyEnabled := pl.IsDMAEnabled()
	if !enabled || dmaAlreadyEnabled {
		if !enabled && dmaAlreadyEnabled {
			pl.dma.Unclaim()
			pl.dma = dmaChannel{} // Invalidate DMA channel.
		}
		return nil
	}

	channel, ok := _DMA.ClaimChannel()
	if !ok {
		return errDMAUnavail
	}

	channel.dl = pl.dma.dl // Copy deadline.
	pl.dma = channel
	cc := pl.dma.CurrentConfig()
	cc.setBSwap(false)
	cc.setTransferDataSize(dmaTxSize8)
	pl.dma.Init(cc)
	return nil
}

func (pl *Parallel8Tx) dmaWrite(data []byte) error {
	dreq := dmaPIO_TxDREQ(pl.sm)
	err := pl.dma.Push8((*byte)(unsafe.Pointer(&pl.sm.TxReg().Reg)), data, dreq)
	if err != nil {
		return err
	}

	// DMA is done after this point but we still have to wait for
	// the FIFO to be empty
	for !pl.sm.IsTxFIFOEmpty() {
		gosched()
	}
	return nil
}
