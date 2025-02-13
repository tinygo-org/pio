//go:build rp2040 || rp2350

package piolib

import (
	"errors"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

var errQueueFull = errors.New("Pulsar:queue full")

// Pulsar implements a square-wave generator that pulses a determined amount of pulses.
type Pulsar struct {
	sm            pio.StateMachine
	offsetPlusOne uint8
}

// NewPulsar returns a new Pulsar ready for use.
func NewPulsar(sm pio.StateMachine, pin machine.Pin) (*Pulsar, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
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

// IsQueueFull checks if the pulsar's queue is full.
func (p *Pulsar) IsQueueFull() bool {
	p.mustValid()
	return p.sm.IsTxFIFOFull()
}

// Queued returns amount of actions in the pulsar's queue.
func (p *Pulsar) Queued() uint8 {
	return uint8(p.sm.TxFIFOLevel())
}

// TryQueue adds an action to the pulsar's queue. If the queue is full it returns an error.
func (p *Pulsar) TryQueue(count uint32) error {
	if count == 0 {
		return nil
	} else if p.IsQueueFull() {
		return errQueueFull
	}
	p.sm.TxPut(count - 1)
	return nil
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

// Stop stops and resets the pulsar to initial state.
// Will unpause pulsar as well if paused and clear it's queue.
func (p *Pulsar) Stop() {
	p.mustValid()
	// See StateMachine.Init for reference on this sequence of operations.
	p.sm.SetEnabled(false)
	p.sm.ClearFIFOs()
	p.sm.Restart()
	p.sm.ClkDivRestart()
	p.sm.Exec(pio.EncodeJmp(p.offsetPlusOne-1, pio.JmpAlways))
	p.sm.SetEnabled(true)
}

func (p *Pulsar) mustValid() {
	if p.offsetPlusOne == 0 {
		panic("piolib: Pulsar not initialized")
	}
}
