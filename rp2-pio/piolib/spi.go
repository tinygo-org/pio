//go:build rp2040

package piolib

import (
	"errors"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type SPI struct {
	sm         pio.StateMachine
	progOffset uint8
	mode       uint8
}

func NewSPI(sm pio.StateMachine, spicfg machine.SPIConfig) (*SPI, error) {
	sm.Claim() // SM should be claimed beforehand, we just guarantee it's claimed.
	const nbits = 8
	// https://github.com/raspberrypi/pico-examples/blob/eca13acf57916a0bd5961028314006983894fc84/pio/spi/spi.pio#L46
	if !sm.IsValid() {
		return nil, errors.New("invalid state machine")
	}

	whole, frac, err := pio.ClkDivFromFrequency(spicfg.Frequency, machine.CPUFrequency())
	if err != nil {
		return nil, err
	}
	Pio := sm.PIO()
	var instructions []uint16
	var origin int8
	var cfger func(uint8) pio.StateMachineConfig
	switch spicfg.Mode {
	case 0b00:
		instructions = spi_cpha0Instructions
		origin = spi_cpha0Origin
		cfger = spi_cpha0ProgramDefaultConfig
	case 0b01:
		// The pin muxes can be configured to invert the output (among other things
		// and this is a cheesy way to get CPOL=1
		// rp.IO_BANK0.GPIO0_CTRL.ReplaceBits(value, ) TODO: https://github.com/raspberrypi/pico-sdk/blob/6a7db34ff63345a7badec79ebea3aaef1712f374/src/rp2_common/hardware_gpio/gpio.c#L80
		// SPI is synchronous, so bypass input synchroniser to reduce input delay.

		instructions = spi_cpha1Instructions
		origin = spi_cpha1Origin
		cfger = spi_cpha1ProgramDefaultConfig
	case 0b10, 0b11:
		return nil, errors.New("unsupported mode")
	default:
		panic("invalid mode")
	}

	offset, err := Pio.AddProgram(instructions, origin)
	if err != nil {
		return nil, err
	}

	cfg := cfger(offset)

	cfg.SetOutPins(spicfg.SDO, 1)
	cfg.SetInPins(spicfg.SDI)
	cfg.SetSidesetPins(spicfg.SCK)

	cfg.SetOutShift(false, true, uint16(nbits))
	cfg.SetInShift(false, true, uint16(nbits))

	cfg.SetClkDivIntFrac(whole, frac)

	// MOSI, SCK output are low, MISO is input.
	outMask := uint32((1 << spicfg.SCK) | (1 << spicfg.SDO))
	sm.SetPinsMasked(0, outMask)
	sm.SetPindirsMasked(outMask, outMask|(1<<spicfg.SDI))

	pincfg := machine.PinConfig{Mode: Pio.PinMode()}
	spicfg.SCK.Configure(pincfg)
	spicfg.SDO.Configure(pincfg)
	spicfg.SDI.Configure(pincfg)
	Pio.HW().INPUT_SYNC_BYPASS.SetBits(1 << spicfg.SDI)

	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	spi := &SPI{sm: sm, progOffset: offset, mode: spicfg.Mode}
	return spi, nil
}

func (spi *SPI) Tx(w, r []byte) error {
	rxRemain, txRemain := len(r), len(w)
	if rxRemain != txRemain {
		return errors.New("expect lengths to be equal")
	}
	retries := int8(32)
	for rxRemain != 0 || txRemain != 0 {
		stall := true
		if txRemain != 0 && !spi.sm.IsTxFIFOFull() {
			spi.sm.TxPut(uint32(w[len(w)-txRemain]))
			txRemain--
			stall = false
		}
		if txRemain != 0 && !spi.sm.IsRxFIFOEmpty() {
			r[len(r)-rxRemain] = uint8(spi.sm.RxGet())
			rxRemain--
			stall = false
		}
		retries--
		if retries <= 0 {
			return errors.New("pioSPI timeout")
		} else if stall {
			// We stalled on this iteration, yield process.
			gosched()
		}
	}
	return nil
}

func (spi *SPI) Transfer(c byte) (rx byte, _ error) {
	waitTx := true
	waitRx := true
	retries := int8(16)
	for waitTx || waitRx {
		if waitTx && !spi.sm.IsTxFIFOFull() {
			spi.sm.TxPut(uint32(c))
			waitTx = false
		}
		if waitRx && !spi.sm.IsRxFIFOEmpty() {
			rx = byte(spi.sm.RxGet())
			waitRx = false
		}
		retries--
		if retries <= 0 {
			return 0, errors.New("pioSPI timeout")
		}
	}
	return rx, nil
}

// SPI represents a SPI bus. It is implemented by the machine.SPI type.
type _SPI interface {
	// Tx transmits the given buffer w and receives at the same time the buffer r.
	// The two buffers must be the same length. The only exception is when w or r are nil,
	// in which case Tx only transmits (without receiving) or only receives (while sending 0 bytes).
	Tx(w, r []byte) error

	// Transfer writes a single byte out on the SPI bus and receives a byte at the same time.
	// If you want to transfer multiple bytes, it is more efficient to use Tx instead.
	Transfer(b byte) (byte, error)
}
