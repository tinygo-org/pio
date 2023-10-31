package piolib

import (
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type I2S struct {
	playBuffer []byte
	sm         pio.StateMachine
	offset     uint8
}

func NewI2S(sm pio.StateMachine, data, clockAndNext machine.Pin) (*I2S, error) {
	sm.Claim() // SM should be claimed beforehand, we just guarantee it's claimed.
	Pio := sm.PIO()

	offset, err := Pio.AddProgram(i2sInstructions, i2sOrigin)
	if err != nil {
		return nil, err
	}
	cfg := i2sProgramDefaultConfig(offset)

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	data.Configure(pinCfg)
	clockAndNext.Configure(pinCfg)
	(clockAndNext + 1).Configure(pinCfg)

	// https://github.com/raspberrypi/pico-extras/blob/09c64d509f1d7a49ceabde699ed6c74c77e195a1/src/rp2_common/pico_audio_i2s/audio_i2s.pio#L48C4-L60C81
	cfg.SetOutPins(data, 1)
	cfg.SetSidesetPins(clockAndNext)
	cfg.SetOutShift(false, true, 32)

	sm.Init(offset, cfg)

	pinMask := uint32(1<<data) | uint32(0b11<<clockAndNext)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.SetPinsMasked(0, pinMask)

	sm.Exec(pio.EncodeJmp(uint16(offset) + i2soffset_entry_point))

	i2s := &I2S{
		sm:     sm,
		offset: offset,
	}
	return i2s, nil
}

func (i2s *I2S) SetSampleFrequency(freq uint32) error {
	freq *= 32 // 32 bits per sample
	whole, frac, err := pio.ClkDivFromFrequency(freq, machine.CPUFrequency())
	if err != nil {
		return err
	}
	i2s.sm.SetClkDiv(whole, frac)
	return nil
}
