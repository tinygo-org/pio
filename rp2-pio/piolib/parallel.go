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
	// BusWidth is the amount of output pins of the parallel bus or 'bits per clock'.
	BusWidth uint8
	// BitsPerPull sets the output shift register (OSR) pull threshold.
	// It determines how many bits to send over bus per value pulled before discarding current OSR value
	// and pulling a new value from TxFIFO.
	// Must be a multiple of BusWidth.
	BitsPerPull uint8
	// ShiftLeft is true if OSR shift direction should be left, false for right shift.
	// For ST7789 and similar displays using 8080 parallel protocol, left shift (ShiftLeft=true)
	// provides correct bit-to-pin mapping where bit 0 maps to the lowest data pin.
	ShiftLeft bool
	// FastMode reduces PIO program size to 2 instructions for higher throughput.
	// This may present instabilities on some systems but should usually "just work".
	// When false, uses a 3-instruction program with additional timing margin.
	FastMode bool
}

func NewParallel(sm pio.StateMachine, cfg ParallelConfig) (*Parallel, error) {
	const sideSetBitCount = 1
	const programOrigin = -1
	asm := pio.AssemblerV0{
		SidesetBits: sideSetBitCount,
	}
	// The ST7789 8080 parallel protocol requires:
	// - Data to be stable before write clock rising edge (V_m constraint)
	// - Clock goes high, then low again for each byte transfer
	//
	// PIO program timing (each instruction = 1 cycle + potential delay):
	// Cycle 0: out pins, <npins>  side 0  (clock LOW, data output begins)
	// Cycle 1: nop               side 1  (clock HIGH, data captured by display)
	// Cycle 2: nop               side 0  (clock LOW again, data setup for next byte)
	//
	// In FastMode (2 instructions), the wrap back to instruction 0 creates the clock LOW period.
	// This satisfies the ST7789 V_m timing requirement (data stable before clock HIGH).
	var rawProgram = [3]uint16{
		asm.Out(pio.OutDestPins, cfg.BusWidth).Side(0).Encode(), //  0: out    pins, <npins>   side 0
		asm.Nop().Side(1).Encode(),                              //  1: nop                    side 1
		asm.Nop().Side(0).Encode(),                              //  2: nop                    side 0
	}
	program := rawProgram[:]
	if cfg.FastMode {
		// Use 2-instruction program for higher throughput.
		// The wrap from instruction 1 back to 0 provides the clock LOW period.
		program = rawProgram[:2]
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
	progOffset, err := Pio.AddProgram(program, programOrigin)
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

	scfg := asm.DefaultStateMachineConfig(progOffset, program)

	scfg.SetOutPins(cfg.DataBase, cfg.BusWidth)
	// ShiftLeft determines OSR shift direction:
	// - ShiftLeft=true: LEFT shift (shift_right=false) - bit 0 goes to lowest pin
	// - ShiftLeft=false: RIGHT shift (shift_right=true) - MSB goes to lowest pin
	// For ST7789 with direct byte-to-pin mapping, ShiftLeft=true is typically needed.
	scfg.SetOutShift(!cfg.ShiftLeft, true, uint16(cfg.BitsPerPull))
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
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

// Tx16 pushes the uint16 buffer to the PIO Tx register.
func (p6 *Parallel) Tx16(data []uint16) (err error) {
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

// Tx8 pushes the uint8 buffer to the PIO Tx register.
func (p6 *Parallel) Tx8(data []uint8) (err error) {
	return helperPushUntilStall(p6.sm, p6.dma, data)
}

func (p6 *Parallel) IsDMAEnabled() bool {
	return p6.dma.helperIsEnabled()
}

func (p6 *Parallel) EnableDMA(enabled bool) error {
	return p6.dma.helperEnableDMA(enabled)
}

// ValidateParallelConfig checks if a ParallelConfig is valid without
// requiring hardware access. Returns nil if valid.
func ValidateParallelConfig(cfg ParallelConfig) error {
	if cfg.BusWidth == 0 {
		return errors.New("zero bus width")
	}
	if cfg.BitsPerPull%cfg.BusWidth != 0 {
		return errors.New("bits per pull must be multiple of bus width")
	}
	if cfg.BitsPerPull < cfg.BusWidth {
		return errors.New("bits per pull must be greater or equal to bus width")
	}
	return nil
}
