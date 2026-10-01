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
	// asyncPending is true while a transfer started by Tx8Async/Tx16Async/
	// Tx32Async has not yet been observed as complete by IsTxAsyncBusy or
	// WaitTxAsync.
	asyncPending bool
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
	if cfg.BusWidth == 0 {
		return nil, errors.New("zero bus width")
	}
	if cfg.BusWidth > 32 {
		return nil, errors.New("bus width exceeds 32 pins")
	}
	pins, err := parallelPinConfig(cfg)
	if err != nil {
		return nil, err
	}

	const sideSetBitCount = 1
	const programOrigin = -1
	asm := pio.AssemblerV0{
		SidesetBits: sideSetBitCount,
	}
	var program = [3]uint16{
		asm.Out(pio.OutDestPins, cfg.BusWidth).Side(0).Encode(), //  0: out    pins, <npins>   side 0
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
	setParallelGPIOBase(Pio, pins.gpioBase)

	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	for pinoff := 0; pinoff < int(cfg.BusWidth); pinoff++ {
		pin := cfg.DataBase + machine.Pin(pinoff)
		pin.Configure(pinCfg)
	}
	cfg.Clock.Configure(pinCfg)

	scfg := asm.DefaultStateMachineConfig(progOffset, program[:])

	scfg.SetOutPins(pins.dataBase, cfg.BusWidth)
	scfg.SetOutShift(true, true, uint16(cfg.BitsPerPull))
	scfg.SetSidesetPins(pins.clock)

	scfg.SetClkDivIntFrac(whole, frac)
	scfg.SetFIFOJoin(pio.FifoJoinTx)

	sm.SetPinsMasked(0, pins.pinMask)
	sm.SetPindirsMasked(pins.pinMask, pins.pinMask)
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
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

// Tx16 pushes the uint16 buffer to the PIO Tx register.
func (p6 *Parallel) Tx16(data []uint16) (err error) {
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

// Tx16 pushes the uint8 buffer to the PIO Tx register.
func (p6 *Parallel) Tx8(data []uint8) (err error) {
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

func (p6 *Parallel) IsDMAEnabled() bool {
	return p6.dma.helperIsEnabled()
}

func (p6 *Parallel) EnableDMA(enabled bool) error {
	return p6.dma.helperEnableDMA(enabled)
}

// Tx32Async starts an asynchronous, DMA-backed transfer of the uint32 buffer
// to the PIO TX register and returns immediately, without waiting for the
// transfer to complete.
//
// EnableDMA(true) must have been called beforehand; Tx32Async returns an
// error if DMA is not enabled. It also returns an error, without starting a
// new transfer, if a previously started async transfer has not yet completed
// (see IsTxAsyncBusy and WaitTxAsync).
//
// data must not be modified, reused for another transfer, or allowed to be
// garbage collected until the transfer completes: the DMA engine reads
// directly from its backing array in the background. Use IsTxAsyncBusy to
// poll for completion, or WaitTxAsync to block until it is done, before
// touching data again or starting another transfer.
func (p6 *Parallel) Tx32Async(data []uint32) error { return parallelTxAsync(p6, data) }

// Tx16Async is the uint16 equivalent of Tx32Async. See Tx32Async for the full
// semantics and buffer-lifetime requirements.
func (p6 *Parallel) Tx16Async(data []uint16) error { return parallelTxAsync(p6, data) }

// Tx8Async is the uint8 equivalent of Tx32Async. See Tx32Async for the full
// semantics and buffer-lifetime requirements.
func (p6 *Parallel) Tx8Async(data []uint8) error { return parallelTxAsync(p6, data) }

// parallelTxAsync implements Tx8Async/Tx16Async/Tx32Async for any supported
// element type.
func parallelTxAsync[T uint8 | uint16 | uint32](p6 *Parallel, data []T) error {
	if p6.asyncPending {
		if helperPushBusy(p6.sm, p6.dma) {
			return errBusy
		}
		p6.asyncPending = false
	}
	if err := helperPushStart(p6.sm, p6.dma, data); err != nil {
		return err
	}
	p6.asyncPending = len(data) != 0
	return nil
}

// IsTxAsyncBusy reports whether a transfer started by Tx8Async, Tx16Async, or
// Tx32Async is still in progress. Completion requires both the DMA channel to
// have finished moving data into the PIO TX FIFO and the state machine to
// have finished shifting that data out to the pins, since the two complete a
// few PIO cycles apart; IsTxAsyncBusy accounts for both.
//
// It is safe to call at any time, including when no async transfer was ever
// started, in which case it returns false.
func (p6 *Parallel) IsTxAsyncBusy() bool {
	if !p6.asyncPending {
		return false
	}
	if helperPushBusy(p6.sm, p6.dma) {
		return true
	}
	p6.asyncPending = false
	return false
}

// WaitTxAsync blocks until a transfer started by Tx8Async, Tx16Async, or
// Tx32Async has fully completed, i.e. until IsTxAsyncBusy would return false.
// It is safe to call even if no async transfer is currently pending.
func (p6 *Parallel) WaitTxAsync() {
	if !p6.asyncPending {
		return
	}
	helperPushWait(p6.sm, p6.dma)
	p6.asyncPending = false
}
