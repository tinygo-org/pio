//go:build rp2040

package piolib

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

// Pulsar implements a square-wave generator that pulses a determined amount of pulses.
type Pulsar struct {
	sm            pio.StateMachine
	offsetPlusOne uint8
}

// NewPulsar returns a new Pulsar ready for use.
func NewPulsar(sm pio.StateMachine, pin machine.Pin) (*Pulsar, error) {
	Pio := sm.PIO()

	offset, err := Pio.AddProgram(pulsarInstructions, pulsarOrigin)
	if err != nil {
		return nil, err
	}
	pin.Configure(machine.PinConfig{Mode: sm.PIO().PinMode()})
	sm.SetPindirsConsecutive(pin, 1, true)
	cfg := pulsarProgramDefaultConfig(offset)
	cfg.SetSetPins(pin, 1)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)
	return &Pulsar{sm: sm, offsetPlusOne: offset + 1}, nil
}

// Start starts the pulsar and does not block.
func (p *Pulsar) Start(count uint32) {
	p.mustValid()
	if count == 0 {
		return
	}
	p.sm.TxPut(count - 1)
}

// SetPeriod sets the pulsar's square-wave period. Is safe to call while pulsar is running.
func (p *Pulsar) SetPeriod(period time.Duration) error {
	p.mustValid()
	period /= 4 // Full pulse cycle is 4 instructions.
	whole, frac, err := pio.ClkDivFromPeriod(uint32(period), uint32(machine.CPUFrequency()))
	if err != nil {
		return err
	}
	p.sm.SetClkDiv(whole, frac)
	return nil
}

// Pause pauses the pulsar if enabled is true. If false unpauses the pulsar.
func (p *Pulsar) Pause(disabled bool) {
	p.mustValid()
	p.sm.SetEnabled(!disabled)
}

// Stop stops and resets the pulsar to initial state. Will unpause pulsar as well if paused.
func (p *Pulsar) Stop() {
	p.mustValid()
	// See StateMachine.Init for reference on this sequence of operations.
	p.sm.SetEnabled(false)
	p.sm.ClearFIFOs()
	p.sm.Restart()
	p.sm.ClkDivRestart()
	p.sm.Exec(pio.EncodeJmp(uint16(p.offsetPlusOne - 1)))
	p.sm.SetEnabled(true)
}

func (p *Pulsar) mustValid() {
	if p.offsetPlusOne == 0 {
		panic("piolib: Pulsar not initialized")
	}
}
