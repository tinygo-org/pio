package piolib

import (
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// Parallel6 implements a 6-parallel bus.
type Parallel6 struct {
	sm        pio.StateMachine
	dma       dmaChannel
	rgboffset uint8
}

// NewParallel6 instantiates a 6-parallel bus with pins dataBase..dataBase+5 and clock pin.
func NewParallel6(sm pio.StateMachine, baud uint32, dataBase machine.Pin, clock machine.Pin) (*Parallel6, error) {
	sm.TryClaim()
	Pio := sm.PIO()
	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	rgbOffset, err := Pio.AddProgram(parallel6Instructions, parallel6Origin)
	if err != nil {
		return nil, err
	}
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	clock.Configure(pinCfg)
	clkMask := uint32(1) << clock
	var pinMask uint32 = clkMask
	for i := 0; i < 6; i++ {
		pin := dataBase + machine.Pin(i)
		pinMask |= 1 << pin
		pin.Configure(pinCfg)
	}

	// Configuration.
	cfg := parallel6ProgramDefaultConfig(rgbOffset)
	// Statemachine Pin configuration.
	cfg.SetOutPins(dataBase, 6)
	cfg.SetOutShift(true, true, 24)
	cfg.SetSidesetPins(clock)

	cfg.SetClkDivIntFrac(whole, frac)
	cfg.SetFIFOJoin(pio.FifoJoinTx)

	sm.SetPinsMasked(0, pinMask)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.Init(rgbOffset, cfg)
	sm.SetEnabled(true)
	p6 := Parallel6{
		sm:        sm,
		rgboffset: rgbOffset,
	}
	return &p6, nil
}

// IsEnabled returns true if the state machine on the Parallel6 is enabled and ready to transmit.
func (p6 *Parallel6) IsEnabled() bool {
	return p6.sm.IsEnabled()
}

// SetEnabled enables or disables the state machine.
func (p6 *Parallel6) SetEnabled(b bool) {
	p6.sm.SetEnabled(b)
}

// Tx24 transmits 6-parallel data over pins. Each 32 bit value contains 24 effective bits
// making a total of 4 clocks.
func (p6 *Parallel6) Tx24(data []uint32) (err error) {
	p6.sm.ClearTxStalled()
	if p6.IsDMAEnabled() {
		err = p6.tx24DMA(data)
	} else {
		err = p6.tx24(data)
	}
	if err != nil {
		return err
	}
	for !p6.sm.HasTxStalled() {
		gosched() // Block until empty.
	}
	return nil
}

func (p6 *Parallel6) tx24(data []uint32) error {
	i := 0
	for i < len(data) {
		if p6.sm.IsTxFIFOFull() {
			gosched()
			continue
		}
		p6.sm.TxPut(data[i])
		i++
	}
	return nil
}

func (p6 *Parallel6) IsDMAEnabled() bool {
	return p6.dma.helperIsEnabled()
}

func (p6 *Parallel6) EnableDMA(enabled bool) error {
	return p6.dma.helperEnableDMA(enabled)
}

func (p6 *Parallel6) tx24DMA(data []uint32) error {
	dreq := dmaPIO_TxDREQ(p6.sm)
	err := p6.dma.Push32(&p6.sm.TxReg().Reg, data, dreq)
	if err != nil {
		return err
	}
	return nil
}
