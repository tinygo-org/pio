//go:build rp2040 || rp2350

package piolib

import (
	"device/rp"
	"machine"
	"runtime/volatile"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// SPI3 is a 3-wire SPI implementation for specialized use cases, such as
// the Pico W's on-board CYW43439 WiFi module. It uses a shared data input/output pin.
type SPI3w struct {
	sm     pio.StateMachine
	dma    dmaChannel
	offset uint8

	statusEn   bool
	lastStatus uint32
	pinMask    uint32
}

func NewSPI3w(sm pio.StateMachine, dio, clk machine.Pin, baud uint32) (*SPI3w, error) {
	baud *= 2 // We have 2 instructions per bit in the hot loop.
	whole, frac, err := pio.ClkDivFromFrequency(baud, machine.CPUFrequency())
	if err != nil {
		return nil, err // Early return on bad clock.
	}

	// https://github.com/embassy-rs/embassy/blob/c4a8b79dbc927e46fcc71879673ad3410aa3174b/cyw43-pio/src/lib.rs#L90
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.

	Pio := sm.PIO()
	offset, err := Pio.AddProgram(spi3wInstructions, spi3wOrigin)
	if err != nil {
		return nil, err
	}

	// Configure state machine.
	cfg := spi3wProgramDefaultConfig(offset)
	cfg.SetOutPins(dio, 1)
	cfg.SetSetPins(dio, 1)
	cfg.SetInPins(dio)
	cfg.SetSidesetPins(clk)
	cfg.SetOutShift(false, true, 32)
	cfg.SetInShift(false, true, 32)
	cfg.SetClkDivIntFrac(whole, frac)

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	dio.Configure(pinCfg)
	clk.Configure(pinCfg)
	Pio.SetInputSyncBypassMasked(1<<dio, 1<<dio)

	dioPad := pinPadCtrl(dio)
	// Disable pull up and pull down.
	dioPad.ReplaceBits(0, 1, rp.PADS_BANK0_GPIO0_PUE_Pos)
	dioPad.ReplaceBits(0, 1, rp.PADS_BANK0_GPIO0_PDE_Pos)

	dioPad.ReplaceBits(1, 1, rp.PADS_BANK0_GPIO0_SCHMITT_Pos) // Enable Schmitt trigger.

	// 12mA drive strength for both clock and output.
	const drive = rp.PADS_BANK0_GPIO0_DRIVE_12mA
	const driveMsk = rp.PADS_BANK0_GPIO0_DRIVE_Msk >> rp.PADS_BANK0_GPIO0_DRIVE_Pos
	dioPad.ReplaceBits(drive, driveMsk, rp.PADS_BANK0_GPIO0_DRIVE_Pos)

	dioPad.ReplaceBits(1, 1, rp.PADS_BANK0_GPIO0_SLEWFAST_Pos) // Enable fast slewrate.

	clkPad := pinPadCtrl(clk)
	clkPad.ReplaceBits(drive, driveMsk, rp.PADS_BANK0_GPIO0_DRIVE_Pos)
	clkPad.ReplaceBits(1, 1, rp.PADS_BANK0_GPIO0_SLEWFAST_Pos) // Enable fast slewrate.

	// Initialize state machine.
	sm.Init(offset, cfg)
	pinMask := uint32(1<<dio | 1<<clk)
	sm.SetPindirsMasked(pinMask, pinMask)
	sm.SetPinsMasked(0, pinMask)

	spiw := &SPI3w{
		sm:      sm,
		offset:  offset,
		pinMask: pinMask,
	}
	return spiw, nil
}

// Tx32 first writes the data in w to the bus and waits until the data is fully sent
// and then reads len(r) 32 bit words from the bus into r. The data exchange is half duplex.
func (spi *SPI3w) Tx32(w, r []uint32) (err error) {
	var writeBits, readBits uint32
	if len(w) > 0 {
		writeBits = uint32(len(w)*32 - 1)
	}
	if len(r) > 0 {
		readBits = uint32(len(r)*32 - 1)
	}
	spi.prepTx(readBits, writeBits)
	deadline := spi.newDeadline()
	if len(w) > 0 {
		err = spi.write(w, deadline)
		if err != nil {
			return err
		}
		err = spi.waitWrite(deadline)
		if err != nil {
			return err
		}
	}
	if len(r) == 0 {
		return nil
	}
	return spi.read(r, deadline)
}

func (spi *SPI3w) CmdWrite(cmd uint32, w []uint32) (err error) {
	writeBits := (1+len(w))*32 - 1
	var readBits uint32
	if spi.statusEn {
		readBits = 31
	}

	spi.prepTx(readBits, uint32(writeBits))
	deadline := spi.newDeadline()
	spi.sm.TxPut(cmd)
	err = spi.write(w, deadline)
	if err != nil {
		return err
	}
	err = spi.waitWrite(deadline)
	if err != nil {
		return err
	}
	if spi.statusEn {
		err = spi.getStatus(deadline)
	}
	return err
}

func (spi *SPI3w) CmdRead(cmd uint32, r []uint32) (err error) {
	const writeBits = 31
	readBits := len(r)*32 - 1
	if spi.statusEn {
		readBits += 32
	}

	spi.prepTx(uint32(readBits), writeBits)
	deadline := spi.newDeadline()
	spi.sm.TxPut(cmd)
	err = spi.read(r, deadline)
	if err != nil {
		return err
	}
	if spi.statusEn {
		err = spi.getStatus(deadline)
	}
	return err
}

func (spi *SPI3w) read(r []uint32, dl deadline) error {
	if spi.IsDMAEnabled() {
		return spi.readDMA(r)
	}
	i := 0
	for i < len(r) {
		if spi.sm.IsRxFIFOEmpty() {
			if dl.expired() {
				return errTimeout
			}
			gosched()
			continue
		}
		r[i] = spi.sm.RxGet()
		spi.sm.TxPut(r[i])
		i++
	}

	return nil
}

func (spi *SPI3w) write(w []uint32, dl deadline) error {
	if spi.IsDMAEnabled() {
		return spi.writeDMA(w)
	}

	i := 0
	for i < len(w) {
		if spi.sm.IsTxFIFOFull() {
			if dl.expired() {
				return errTimeout
			}
			gosched()
			continue
		}
		spi.sm.TxPut(w[i])
		i++
	}
	return nil
}

func (spi *SPI3w) waitWrite(deadline deadline) error {
	// DMA/TxPush is done after this point but we still have to wait for
	// the FIFO to be empty.
	for !spi.sm.IsTxFIFOEmpty() {
		if deadline.expired() {
			return errTimeout
		}
		gosched()
	}
	return nil
}

// LastStatus returns the latest status. This is only valid if EnableStatus(true) was called.
func (spi *SPI3w) LastStatus() uint32 {
	return spi.lastStatus
}

// EnableStatus enables the reading of the last status word after a CmdRead/CmdWrite.
func (spi *SPI3w) EnableStatus(enabled bool) {
	spi.statusEn = enabled
}

// SetTimeout sets the read/write timeout. Use 0 as argument to disable timeouts.
func (spi *SPI3w) SetTimeout(timeout time.Duration) {
	spi.dma.dl.setTimeout(timeout)
}

func (spi *SPI3w) newDeadline() deadline {
	return spi.dma.dl.newDeadline()
}

func (spi *SPI3w) getStatus(dl deadline) error {
	for spi.sm.IsRxFIFOEmpty() {
		if dl.expired() {
			return errTimeout
		}
		gosched()
	}

	err := spi.read(unsafe.Slice(&spi.lastStatus, 1), dl)
	if err != nil {
		return err
	}
	return nil
}

func (spi *SPI3w) prepTx(readbits, writebits uint32) {
	spi.sm.SetEnabled(false)
	// Clearing the FIFO will prevent remaining data from leaving
	// a HIGH on the data pin apparently.
	spi.sm.ClearFIFOs()
	// The state machine must be restarted to prevent glitchiness.
	spi.sm.Restart()

	spi.sm.SetX(writebits)
	spi.sm.SetY(readbits)
	spi.sm.Exec(pio.EncodeSet(pio.SrcDestPinDirs, 1)) // Set Pindir out.
	spi.sm.Jmp(spi.offset+spi3wWrapTarget, pio.JmpAlways)

	spi.sm.SetEnabled(true)
}

// DMA code below.

func (spi *SPI3w) EnableDMA(enabled bool) error {
	return spi.dma.helperEnableDMA(enabled)
}

func (spi *SPI3w) readDMA(r []uint32) error {
	dreq := dmaPIO_RxDREQ(spi.sm)
	err := spi.dma.Pull32(r, &spi.sm.RxReg().Reg, dreq)
	if err != nil {
		return err
	}
	return nil
}

func (spi *SPI3w) writeDMA(w []uint32) error {
	dreq := dmaPIO_TxDREQ(spi.sm)
	err := spi.dma.Push32(&spi.sm.TxReg().Reg, w, dreq)
	if err != nil {
		return err
	}
	return nil
}

func (spi *SPI3w) IsDMAEnabled() bool {
	return spi.dma.helperIsEnabled()
}

func pinPadCtrl(pin machine.Pin) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(uintptr(unsafe.Pointer(&rp.PADS_BANK0.GPIO0)) + uintptr(4*pin)))
}
