package piolib

import (
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type ParallelGeneric struct {
	sm      pio.StateMachine
	progOff uint8
	dma     dmaChannel
}

type ParallelGenericConfig struct {
	Baud      uint32
	DataBase  machine.Pin
	Clock     machine.Pin
	BusWidth  uint8
	ShiftBits uint8
	// Maybe add Slewrate, drive strength among other parameters in future.
}

func NewParallelGeneric(sm pio.StateMachine, cfg ParallelGenericConfig) (*ParallelGeneric, error) {
	sm.TryClaim()
	Pio := sm.PIO()
	whole, frac, err := pio.ClkDivFromFrequency(cfg.Baud, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	const sideSetBitCount = 1
	const programOrigin = -1
	var program = [3]uint16{ // modifying this length means yoy
		pio.EncodeOut(pio.SrcDestOSR, cfg.ShiftBits) | pio.EncodeSideSet(sideSetBitCount, 0), //  0: out    pins, <shift>   side 0
		pio.EncodeNOP() | pio.EncodeSideSet(sideSetBitCount, 1),                              //  1: nop                    side 1
		pio.EncodeNOP() | pio.EncodeSideSet(sideSetBitCount, 0),                              //  2: nop                    side 0
	}
	progOffset, err := Pio.AddProgram(program[:], programOrigin)
	if err != nil {
		return nil, err
	}

	clkMask := uint32(1) << cfg.Clock
	pinMask := clkMask
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	for pinoff := 0; pinoff < int(cfg.BusWidth); pinoff++ {
		pin := cfg.DataBase + machine.Pin(pinoff)
		pinMask |= 1 << pin
		pin.Configure(pinCfg)
	}

	scfg := pio.DefaultStateMachineConfig()
	{ // parallelGenericProgramDefaultConfig
		scfg.SetWrap(progOffset+0, progOffset+uint8(len(program))-1)
		scfg.SetSidesetParams(1, false, false)
	}

	scfg.SetOutPins(cfg.DataBase, cfg.BusWidth)
	scfg.SetOutShift(true, true, uint16(cfg.ShiftBits))
	scfg.SetSidesetPins(cfg.Clock)

	scfg.SetClkDivIntFrac(whole, frac)
	scfg.SetFIFOJoin(pio.FifoJoinTx)

	sm.SetPinsMasked(0, pinMask)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.Init(progOffset, scfg)
	sm.SetEnabled(true)
	return &ParallelGeneric{
		sm:      sm,
		progOff: progOffset,
	}, nil
}

// IsEnabled returns true if the state machine on the Parallel6 is enabled and ready to transmit.
func (p6 *ParallelGeneric) IsEnabled() bool {
	return p6.sm.IsEnabled()
}

// SetEnabled enables or disables the state machine.
func (p6 *ParallelGeneric) SetEnabled(b bool) {
	p6.sm.SetEnabled(b)
}

// Tx32 pushes the uint32 buffer to the PIO Tx register.
func (p6 *ParallelGeneric) Tx32(data []uint32) (err error) {
	p6.sm.ClearTxStalled()
	if p6.IsDMAEnabled() {
		err = p6.tx32DMA(data)
	} else {
		err = p6.tx32(data)
	}
	if err != nil {
		return err
	}
	for !p6.sm.HasTxStalled() {
		gosched() // Block until empty.
	}
	return nil
}

func (p6 *ParallelGeneric) tx32(data []uint32) error {
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

func (p6 *ParallelGeneric) IsDMAEnabled() bool {
	return p6.dma.helperIsEnabled()
}

func (p6 *ParallelGeneric) EnableDMA(enabled bool) error {
	return p6.dma.helperEnableDMA(enabled)
}

func (p6 *ParallelGeneric) tx32DMA(data []uint32) error {
	dreq := dmaPIO_TxDREQ(p6.sm)
	err := p6.dma.Push32(&p6.sm.TxReg().Reg, data, dreq)
	if err != nil {
		return err
	}
	return nil
}
