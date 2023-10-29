//go:generate pioasm -o go pulsar.pio pulsar_pio.go
package main

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

func main() {
	time.Sleep(time.Second)
	const pin = machine.GP15
	pulsar, err := NewPulsar(pio.PIO0.StateMachine(0), pin)
	if err != nil {
		panic(err.Error())
	}
	println("start pulsing")

	for {
		// Max period is 0.5ms. PIO state machines can run at minimum of 2kHz.
		for period := time.Microsecond; period < time.Millisecond/3; period *= 2 {
			err = pulsar.SetPeriod(period)
			if err != nil {
				panic(err.Error())
			}
			for i := uint32(10); i < 100; i *= 2 {
				pulsar.Start(i)
				time.Sleep(time.Second / 2)
			}
		}
	}
}

type Pulsar struct {
	sm     pio.StateMachine
	offset uint8
}

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
	return &Pulsar{sm: sm, offset: offset}, nil
}

func (p *Pulsar) Start(count uint32) {
	if count == 0 {
		return
	}
	p.sm.TxPut(count - 1)
}

func (p *Pulsar) SetPeriod(period time.Duration) error {
	period /= 4 // Full pulse cycle is 4 instructions.
	whole, frac, err := pio.ClkDivFromPeriod(uint32(period), uint32(machine.CPUFrequency()))
	if err != nil {
		return err
	}
	p.sm.SetClkDiv(whole, frac)
	return nil
}

func (p *Pulsar) Pause(enabled bool) {
	p.sm.SetEnabled(enabled)
}

func (p *Pulsar) Stop() {
	// See StateMachine.Init for reference on this sequence of operations.
	p.sm.SetEnabled(false)
	p.sm.ClearFIFOs()
	p.sm.Restart()
	p.sm.ClkDivRestart()
	p.sm.Exec(pio.EncodeJmp(uint16(p.offset)))
	p.sm.SetEnabled(true)
}
