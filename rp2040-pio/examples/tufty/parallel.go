package main

import (
	"device/rp"
	"errors"
	"machine"
	"runtime"
	"runtime/volatile"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

type pioParallel struct {
	sm         pio.StateMachine
	dmaChannel uint32
}

const noDMA uint32 = 0xffff_ffff

func NewPIOParallel(sm pio.StateMachine, dStart, wr machine.Pin) (*pioParallel, error) {
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(st7789_parallelInstructions, st7789_parallelOrigin)
	if err != nil {
		return nil, err
	}
	parallelST7789Init(sm, offset, dStart, wr)

	sm.SetEnabled(true)

	return &pioParallel{sm: sm, dmaChannel: noDMA}, nil
}

func (pl *pioParallel) Write(data []uint8) {
	if pl.dmaChannel != noDMA {
		pl.dmaWrite(data)
		return
	}
	// Add separate control flow for when using DMA.
	for _, char := range data {
		pl.sm.TxPut(uint32(char))

		retries := int8(32)
		for pl.sm.IsTxFIFOFull() && retries > 0 {
			println("Waiting for FIFO to empty")
			runtime.Gosched()
			retries--
		}
		if retries <= 0 {
			println("FIFO never emptied")
		}
	}
}

func (pl *pioParallel) EnableDMA(dmaChan uint32) error {
	if !pl.sm.IsValid() {
		return errors.New("PIO Statemachine needs initializing") //Not initialized
	}
	pioHW := pl.sm.PIO().HW()
	dmaConfig := getDefaultDMAConfig(dmaChan)
	setTransferDataSize(dmaConfig, DMA_SIZE_8)
	setBSwap(dmaConfig, false)
	setDREQ(dmaConfig, pioHW.GetIRQ())
	dmaChannelConfigure(dmaChan, dmaConfig, pl.sm.TxReg(), nil, 0, false)
	return nil
}

func (pl *pioParallel) dmaWrite(data []byte) {
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
