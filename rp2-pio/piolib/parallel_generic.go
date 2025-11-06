package piolib

import (
	"errors"
	"machine"
	"math"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// Parallel implements a parallel bus of arbitrary number of data lines (up to 32).
type Parallel struct {
	sm      pio.StateMachine
	progOff uint8
	dma     dmaChannel
}

type ParallelConfig struct {
	// Baud determines the clock speed of the parallel bus.
	Baud uint32
	// Clock is the single clock pin for the parallel bus.
	Clock machine.Pin
	// DataBase is the first of BusWidth consecutive pins defining the data lines of the parallel bus.
	DataBase machine.Pin
	// BusWidth is the amount of output pins of the parallel bus.
	BusWidth uint8
	// BitsPerPull sets the output shift register (OSR) pull threshold.
	// It determines how many bits to send over bus per value pulled before discarding current OSR value
	// and pulling a new value from TxFIFO.
	// Must be a multiple of BusWidth.
	BitsPerPull uint8
}

func NewParallel(sm pio.StateMachine, cfg ParallelConfig) (*Parallel, error) {
	const sideSetBitCount = 1
	const programOrigin = -1
	asm := pio.Assembler{
		SidesetBits: sideSetBitCount,
	}
	var program = [3]uint16{
		asm.Out(pio.SrcDestPins, cfg.BusWidth).Side(0).Encode(), //  0: out    pins, <npins>   side 0
		asm.Nop().Side(1).Encode(),                              //  1: nop                    side 1
		asm.Nop().Side(0).Encode(),                              //  2: nop                    side 0
	}
	maxBaud := math.MaxUint32 / uint32(len(program))
	if cfg.Baud > maxBaud {
		return nil, errors.New("max baud for parallel exceeded")
	} else if cfg.BitsPerPull%cfg.BusWidth != 0 {
		return nil, errors.New("bits per pull must be multiple of bus width")
	} else if cfg.BitsPerPull < cfg.BusWidth {
		return nil, errors.New("bits per pull must be greater or equal to bus width")
	} else if cfg.BusWidth == 0 {
		return nil, errors.New("zero bus width")
	}
	piofreq := cfg.Baud * uint32(len(program))
	whole, frac, err := pio.ClkDivFromFrequency(piofreq, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}

	sm.TryClaim()
	Pio := sm.PIO()
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
	cfg.Clock.Configure(pinCfg)

	scfg := pio.DefaultStateMachineConfig()
	{ // parallelGenericProgramDefaultConfig
		scfg.SetWrap(progOffset+0, progOffset+uint8(len(program))-1)
		scfg.SetSidesetParams(1, false, false)
	}

	scfg.SetOutPins(cfg.DataBase, cfg.BusWidth)
	scfg.SetOutShift(true, true, uint16(cfg.BitsPerPull))
	scfg.SetSidesetPins(cfg.Clock)

	scfg.SetClkDivIntFrac(whole, frac)
	scfg.SetFIFOJoin(pio.FifoJoinTx)

	sm.SetPinsMasked(0, pinMask)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.Init(progOffset, scfg)
	sm.SetEnabled(true)
	return &Parallel{
		sm:      sm,
		progOff: progOffset,
	}, nil
}

// IsEnabled returns true if the state machine on the Parallel6 is enabled and ready to transmit.
func (p6 *Parallel) IsEnabled() bool {
	return p6.sm.IsEnabled()
}

// SetEnabled enables or disables the state machine.
func (p6 *Parallel) SetEnabled(b bool) {
	p6.sm.SetEnabled(b)
}

// Tx32 pushes the uint32 buffer to the PIO Tx register.
func (p6 *Parallel) Tx32(data []uint32) (err error) {
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

func (p6 *Parallel) tx32(data []uint32) error {
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

func (p6 *Parallel) IsDMAEnabled() bool {
	return p6.dma.helperIsEnabled()
}

func (p6 *Parallel) EnableDMA(enabled bool) error {
	return p6.dma.helperEnableDMA(enabled)
}

func (p6 *Parallel) tx32DMA(data []uint32) error {
	dreq := dmaPIO_TxDREQ(p6.sm)
	err := p6.dma.Push32(&p6.sm.TxReg().Reg, data, dreq)
	if err != nil {
		return err
	}
	return nil
}
