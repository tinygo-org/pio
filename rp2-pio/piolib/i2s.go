//go:build rp2040 || rp2350

package piolib

import (
	"errors"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// I2S is a wrapper around a PIO state machine that implements I2S.
// Currently only supports writing to the I2S peripheral.
type I2S struct {
	sm      pio.StateMachine
	offset  uint8
	writing bool
}

// NewI2S creates a new I2S peripheral using the given PIO state machine.
func NewI2S(sm pio.StateMachine, data, clockAndNext machine.Pin) (*I2S, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
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
	sm.Jmp(offset+i2soffset_entry_point, pio.JmpAlways)

	i2s := &I2S{
		sm:     sm,
		offset: offset,
	}
	// This enables the state machine. Good practice to not require users to do this
	// since they may be confused why nothing is happening.
	i2s.Enable(true)

	return i2s, nil
}

// SetSampleFrequency sets the sample frequency of the I2S peripheral.
func (i2s *I2S) SetSampleFrequency(freq uint32) error {
	freq *= 32 // 32 bits per sample
	whole, frac, err := pio.ClkDivFromFrequency(freq, machine.CPUFrequency())
	if err != nil {
		return err
	}
	i2s.sm.SetClkDiv(whole, frac)
	return nil
}

// WriteMono writes a mono audio buffer to the I2S peripheral.
func (i2s *I2S) WriteMono(b []uint16) (int, error) {
	return i2sWrite(i2s, b)
}

// WriteStereo writes a stereo audio buffer to the I2S peripheral.
func (i2s *I2S) WriteStereo(b []uint32) (int, error) {
	return i2sWrite(i2s, b)
}

// ReadMono reads a mono audio buffer from the I2S peripheral.
func (i2s *I2S) ReadMono(p []uint16) (n int, err error) {
	return 0, errors.ErrUnsupported
}

// ReadStereo reads a stereo audio buffer from the I2S peripheral.
func (i2s *I2S) ReadStereo(p []uint32) (n int, err error) {
	return 0, errors.ErrUnsupported
}

func i2sWrite[T uint16 | uint32](i2s *I2S, b []T) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if i2s.writing {
		return 0, errBusy
	}
	i2s.writing = true
	i := 0
	for i < len(b) {
		if i2s.sm.IsTxFIFOFull() {
			gosched()
			continue
		} else if !i2s.writing {
			return i, nil
		}
		i2s.sm.TxPut(uint32(b[i]))
		i++
	}
	i2s.writing = false
	return len(b), nil
}

// Enable enables or disables the I2S peripheral.
func (i2s *I2S) Enable(enabled bool) {
	i2s.sm.SetEnabled(enabled)
}
