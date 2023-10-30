//go:build rp2040

package piolib

import (
	"errors"
	"machine"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

// Parallel8Tx is a 8-wire, only send Parallel implementation.
type Parallel8Tx struct {
	sm         pio.StateMachine
	offset     uint8
	dmaChannel uint32
}

// unused for now.
const noDMA uint32 = 0xffff_ffff

func NewParallel8Tx(sm pio.StateMachine, wr, dStart machine.Pin, baud uint32) (*Parallel8Tx, error) {
	const nPins = 8
	if dStart+nPins > 31 {
		return nil, errors.New("invalid D0..D7 pin range")
	}
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(parallel8Instructions, parallel8Origin)
	if err != nil {
		return nil, err
	}

	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	for i := dStart; i < dStart+nPins; i++ {
		i.Configure(pinCfg)
	}
	wr.Configure(pinCfg)

	sm.SetPindirsConsecutive(dStart, nPins, true)

	cfg := parallel8ProgramDefaultConfig(offset)

	cfg.SetOutPins(dStart, nPins)
	cfg.SetSidesetPins(wr)
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	cfg.SetOutShift(false, true, nPins)

	baud *= 4 // Parallel is 4 instructions, so we need to multiply baud by 4 to get PIO frequency.
	whole, frac, err := pio.ClkDivFromPeriod(1e9/baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	cfg.SetClkDivIntFrac(whole, frac)

	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	return &Parallel8Tx{sm: sm, dmaChannel: noDMA, offset: offset}, nil
}

func (pl *Parallel8Tx) Write(data []uint8) error {
	if pl.dmaChannel != noDMA {
		// pl.dmaWrite(data)
		return nil
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

/*
func (pl *Parallel8Tx) EnableDMA(dmaChan uint32) error {
	if !pl.sm.IsValid() {
		return errors.New("PIO Statemachine needs initializing") //Not initialized
	}
	pl.dmaChannel = dmaChan // DMA enabled
	if dmaChan == noDMA {
		return nil
	}
	Pio := pl.sm.PIO()
	dmaConfig := getDefaultDMAConfig(dmaChan)
	setTransferDataSize(dmaConfig, DMA_SIZE_8)
	setBSwap(dmaConfig, false)
	setDREQ(dmaConfig, uint32(Pio.GetIRQ()))
	dmaChannelConfigure(dmaChan, dmaConfig, pl.sm.TxReg(), nil, 0, false)
	return nil
}

func (pl *Parallel8Tx) dmaWrite(data []byte) {
	dmaChan := &dmaChannels[pl.dmaChannel]
	for dmaChan.CTRL_TRIG.Get()&rp.DMA_CH0_CTRL_TRIG_BUSY != 0 {
		runtime.Gosched()
	}

	readAddr := uint32(uintptr(unsafe.Pointer(&data[0])))
	dmaChan.TRANS_COUNT.Set(uint32(len(data)))
	dmaChan.READ_ADDR.Set(uint32(readAddr))
	dmaChan.CTRL_TRIG.Set(dmaChan.CTRL_TRIG.Get() | rp.DMA_CH0_CTRL_TRIG_EN)

	for dmaChan.CTRL_TRIG.Get()&rp.DMA_CH0_CTRL_TRIG_BUSY != 0 {
		runtime.Gosched()
	}
	for !pl.sm.IsTxFIFOEmpty() {
		runtime.Gosched()
	}
}

func dmaChannelConfigure(channel, config uint32, writeAddr, readAddr *volatile.Register32, transferCount uint32, trigger bool) {
	regAddr := func(reg *volatile.Register32) uint32 {
		if reg == nil {
			return 0
		}
		return uint32(uintptr(unsafe.Pointer(reg)))
	}
	dmaChan := dmaChannels[channel]
	dmaChan.READ_ADDR.Set(regAddr(readAddr))
	dmaChan.WRITE_ADDR.Set(regAddr(writeAddr))
	dmaChan.TRANS_COUNT.Set(transferCount)
	dmaChan.CTRL_TRIG.Set(config | rp.DMA_CH0_CTRL_TRIG_EN)
}
*/
