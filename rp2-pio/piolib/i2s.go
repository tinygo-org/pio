//go:build rp2040 || rp2350

package piolib

import (
	"errors"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// I2S is a wrapper around a PIO state machine that implements I2S.
// Currently only supports writing to the I2S peripheral.
type I2S struct {
	sm        pio.StateMachine
	offset    uint8
	fifoSleep uint32
}

// NewI2S creates a new I2S peripheral using the given PIO state machine.
func NewI2S(sm pio.StateMachine, data, clockAndNext machine.Pin) (*I2S, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	Pio := sm.PIO()

	// Program positions.
	const (
		origin     = -1
		entryPoint = 7
		bitloop1   = 0
		bitloop0   = 4
	)
	// Sideset pin mapping: bit0=BCLK (bit clock), bit1=LRCLK (left/right channel select)
	// LRCLK=1 for left channel, LRCLK=0 for right channel (I2S standard)
	// Each loop outputs 16 bits per channel (1 initial + 15 in loop), 32 bits total per stereo sample
	asm := pio.AssemblerV0{SidesetBits: 2}
	var program = [...]uint16{
		//     .wrap_target
		bitloop1:// Left channel (LRCLK=1): output 16 bits with BCLK toggling
		asm.Out(pio.OutDestPins, 1).Side(0b10).Encode(), // 0: out  pins, 1  BCLK=0, LRCLK=1
		asm.Jmp(pio.JmpXNZeroDec, bitloop1).Side(0b11).Encode(), // 1: jmp  x--, 0   BCLK=1, LRCLK=1
		asm.Out(pio.OutDestPins, 1).Side(0b00).Encode(),         // 2: out  pins, 1  BCLK=0, LRCLK=0 (transition to right)
		asm.Set(pio.SetDestX, 14).Side(0b01).Encode(),           // 3: set  x, 14    BCLK=1, LRCLK=0

		bitloop0:// Right channel (LRCLK=0): output 16 bits with BCLK toggling
		asm.Out(pio.OutDestPins, 1).Side(0b00).Encode(), // 4: out  pins, 1  BCLK=0, LRCLK=0
		asm.Jmp(pio.JmpXNZeroDec, bitloop0).Side(0b01).Encode(), // 5: jmp  x--, 4   BCLK=1, LRCLK=0
		asm.Out(pio.OutDestPins, 1).Side(0b10).Encode(),         // 6: out  pins, 1  BCLK=0, LRCLK=1 (transition to left)
		asm.Set(pio.SetDestX, 14).Side(0b11).Encode(),           // 7: set  x, 14    BCLK=1, LRCLK=1
		//     .wrap
	}

	offset, err := Pio.AddProgram(program[:], origin)
	if err != nil {
		return nil, err
	}
	cfg := asm.DefaultStateMachineConfig(offset, program[:])

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	data.Configure(pinCfg)
	clockAndNext.Configure(pinCfg)
	(clockAndNext + 1).Configure(pinCfg)

	// https://github.com/raspberrypi/pico-extras/blob/09c64d509f1d7a49ceabde699ed6c74c77e195a1/src/rp2_common/pico_audio_i2s/audio_i2s.pio#L48C4-L60C81
	cfg.SetOutPins(data, 1)
	cfg.SetSidesetPins(clockAndNext)
	cfg.SetOutShift(false, true, 32)

	// Join the RX and TX FIFOs into a single TX FIFO of depth 8.
	// This will likely need removing if reads get implemented,
	// but it's just an optimization, it shouldn't break anything.
	cfg.SetFIFOJoin(pio.FifoJoinTx)

	sm.Init(offset, cfg)

	pinMask := uint32(1<<data) | uint32(0b11<<clockAndNext)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.SetPinsMasked(0, pinMask)
	sm.Jmp(pio.JmpAlways, offset+entryPoint)

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

	// Each FIFO entry is one 32-bit word.  Each audio frame is also
	// one 32-bit word.  Calculate how long that takes to play so we
	// can sleep for long enough to clear a FIFO entry to write into
	// whenever we have frames to write but the FIFO is full.
	frameTime := uint32(time.Second) / freq
	// N.B. time.Second < math.MaxUint32 => this never overflows
	// (frameTime == 22675ns @ 44.1kHz, 20833ns @ 48kHz)

	// Sleep for at least 1 bit of audio output longer, so we never
	// end our sleep exactly as the final bit is output and so end up
	// sleeping a whole other frame.
	i2s.fifoSleep = frameTime + frameTime/32

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
	// You probably need to remove the cfg.SetFIFOJoin in NewI2S if
	// you're implementing reading or you won't have a FIFO to read
	// into.
	return 0, errors.ErrUnsupported
}

// ReadStereo reads a stereo audio buffer from the I2S peripheral.
func (i2s *I2S) ReadStereo(p []uint32) (n int, err error) {
	// You probably need to remove the cfg.SetFIFOJoin in NewI2S if
	// you're implementing reading or you won't have a FIFO to read
	// into.
	return 0, errors.ErrUnsupported
}

func i2sWrite[T uint16 | uint32](i2s *I2S, b []T) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	i := 0
	for i < len(b) {
		if i2s.sm.IsTxFIFOFull() {
			time.Sleep(time.Duration(i2s.fifoSleep))
			continue
		}
		i2s.sm.TxPut(uint32(b[i]))
		i++
	}
	return len(b), nil
}

// Enable enables or disables the I2S peripheral.
func (i2s *I2S) Enable(enabled bool) {
	i2s.sm.SetEnabled(enabled)
}
