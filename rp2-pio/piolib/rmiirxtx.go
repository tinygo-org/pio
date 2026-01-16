//go:build rp2040 || rp2350

package piolib

import (
	"device/rp"
	"errors"
	"machine"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// RMIITxRx is the Reduced Media Independent Interface for 100Mbps Ethernet PHY communication.
// Uses DMA for both TX and RX. RX uses GPIO interrupt on CRS_DV falling edge.
// Inspired by Sandeep Mistry's pico-rmii-ethernet implementation.
type RMIITxRx struct {
	smTx         pio.StateMachine
	smRx         pio.StateMachine
	programOffTx uint8
	programOffRx uint8
	dmaTx        dmaChannel
	dmaRx        dmaChannel
	crsdvPin     machine.Pin
	rxBuf        []byte
	rxCallback   func(n int)
}

// RMIITxRxConfig configures the RMII interface pins.
type RMIITxRxConfig struct {
	// Baud is the clock divider for PIO state machines.
	Baud uint32
	// TxPin is the base pin for RMII TX (TXD0, TXD1, TX_EN). Requires 3 consecutive pins.
	TxPin machine.Pin
	// RxPin is the base pin for RMII RX (RXD0, RXD1). Requires 2 consecutive pins.
	RxPin machine.Pin
	// CRSDVPin is the Carrier Sense/Data Valid pin.
	CRSDVPin machine.Pin
	// RefClkPin is the 50MHz reference clock input from PHY.
	RefClkPin machine.Pin
}

// NewRMIITxRx creates a new RMII interface. Both state machines must be from the same PIO block.
func NewRMIITxRx(smTx, smRx pio.StateMachine, cfg RMIITxRxConfig) (*RMIITxRx, error) {
	if smTx.PIO().BlockIndex() != smRx.PIO().BlockIndex() {
		return nil, errors.New("TX and RX state machines must be from same PIO block")
	}

	smTx.TryClaim()
	smRx.TryClaim()

	Pio := smTx.PIO()
	var asm pio.AssemblerV0

	// RX Program: Wait for sync, then read 2 bits continuously
	const rxWrapTarget = 6
	rxProgram := [7]uint16{
		asm.WaitPin(false, 2).Encode(), // wait 0 pin 2 (CRSDV)
		asm.WaitPin(false, 0).Encode(), // wait 0 pin 0 (RXD0)
		asm.WaitPin(false, 1).Encode(), // wait 0 pin 1 (RXD1)
		asm.WaitPin(true, 2).Encode(),  // wait 1 pin 2 (CRSDV)
		asm.WaitPin(true, 0).Encode(),  // wait 1 pin 0 (RXD0)
		asm.WaitPin(true, 1).Encode(),  // wait 1 pin 1 (RXD1)
		rxWrapTarget:                   asm.In(pio.InSrcPins, 2).Encode(), // in pins, 2
	}

	rxOffset, err := Pio.AddProgram(rxProgram[:], -1)
	if err != nil {
		return nil, err
	}

	// TX Program: Output 3 pins (TXD0, TXD1, TX_EN)
	txProgram := [1]uint16{
		asm.Out(pio.OutDestPins, 3).Encode(),
	}

	txOffset, err := Pio.AddProgram(txProgram[:], -1)
	if err != nil {
		Pio.ClearProgramSection(rxOffset, uint8(len(rxProgram)))
		return nil, err
	}

	// Calculate clock divider from baud rate
	whole, frac, err := pio.ClkDivFromFrequency(cfg.Baud, machine.CPUFrequency())
	if err != nil {
		Pio.ClearProgramSection(rxOffset, uint8(len(rxProgram)))
		Pio.ClearProgramSection(txOffset, uint8(len(txProgram)))
		return nil, err
	}

	// Claim DMA channels
	dmaTx, ok := _DMA.ClaimChannel()
	if !ok {
		return nil, errors.New("no DMA channel available for TX")
	}
	dmaRx, ok := _DMA.ClaimChannel()
	if !ok {
		dmaTx.Unclaim()
		return nil, errors.New("no DMA channel available for RX")
	}

	// Configure RX state machine
	rxcfg := pio.DefaultStateMachineConfig()
	rxcfg.SetWrap(rxOffset+rxWrapTarget, rxOffset+uint8(len(rxProgram))-1)
	rxcfg.SetInPins(cfg.RxPin, 2)
	rxcfg.SetInShift(true, true, 8) // Shift right, autopush, 8 bits (like Sandeep)
	rxcfg.SetClkDivIntFrac(whole, frac)
	rxcfg.SetFIFOJoin(pio.FifoJoinRx)

	// Configure TX state machine
	txcfg := pio.DefaultStateMachineConfig()
	txcfg.SetWrap(txOffset, txOffset+uint8(len(txProgram))-1)
	txcfg.SetOutPins(cfg.TxPin, 3)
	txcfg.SetOutShift(true, true, 8) // Shift right, autopull, 8 bits
	txcfg.SetClkDivIntFrac(whole, frac)
	txcfg.SetFIFOJoin(pio.FifoJoinTx)

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}
	for i := 0; i < 3; i++ {
		(cfg.TxPin + machine.Pin(i)).Configure(pinCfg)
	}
	cfg.RxPin.Configure(pinCfg)
	(cfg.RxPin + 1).Configure(pinCfg)
	cfg.CRSDVPin.Configure(pinCfg)
	cfg.RefClkPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	// Set TX pins as output
	txPinMask := uint32(0b111 << cfg.TxPin)
	smTx.SetPindirsMasked(txPinMask, txPinMask)
	smTx.SetPinsMasked(0, txPinMask)

	// Set RX pins as input
	rxPinMask := uint32(0b11<<cfg.RxPin | 1<<cfg.CRSDVPin)
	smRx.SetPindirsMasked(0, rxPinMask)

	// Initialize state machines (but don't enable yet)
	smRx.Init(rxOffset, rxcfg)
	smTx.Init(txOffset, txcfg)
	smTx.SetEnabled(true)

	return &RMIITxRx{
		smTx:         smTx,
		smRx:         smRx,
		programOffTx: txOffset,
		programOffRx: rxOffset,
		dmaTx:        dmaTx,
		dmaRx:        dmaRx,
		crsdvPin:     cfg.CRSDVPin,
	}, nil
}

// SetRxHandler sets the receive buffer and callback.
// Callback is called with the number of bytes received when CRS_DV falls.
func (r *RMIITxRx) SetRxHandler(buf []byte, callback func(n int)) {
	r.rxBuf = buf
	r.rxCallback = callback
}

// StartRx starts receiving into the buffer set by SetRxHandler.
// Non-blocking. Callback fires when frame ends (CRS_DV falls).
func (r *RMIITxRx) StartRx() error {
	if r.rxBuf == nil || r.rxCallback == nil {
		return errors.New("call SetRxHandler first")
	}

	// Clear buffer
	for i := range r.rxBuf {
		r.rxBuf[i] = 0
	}

	// Configure DMA: PIO RX FIFO -> rxBuf
	hw := r.dmaRx.HW()
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	hw.READ_ADDR.Set(uint32(uintptr(unsafe.Pointer(&r.smRx.RxReg().Reg))) + 3) // +3 for byte access
	hw.WRITE_ADDR.Set(uint32(uintptr(unsafe.Pointer(&r.rxBuf[0]))))
	hw.TRANS_COUNT.Set(uint32(len(r.rxBuf)))

	cc := r.dmaRx.CurrentConfig()
	cc.setTREQ_SEL(dmaPIO_RxDREQ(r.smRx))
	cc.setTransferDataSize(dmaTxSize8)
	cc.setChainTo(r.dmaRx.idx)
	cc.setReadIncrement(false)
	cc.setWriteIncrement(true)
	cc.setEnable(true)
	hw.CTRL_TRIG.Set(cc.CTRL)

	// Re-init and enable RX state machine
	r.smRx.SetEnabled(false)
	r.smRx.ClearFIFOs()
	r.smRx.Restart()
	r.smRx.Jmp(pio.JmpAlways, r.programOffRx)
	r.smRx.SetEnabled(true)

	// Set GPIO interrupt for CRS_DV falling edge
	r.crsdvPin.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
		r.onRxComplete()
	})

	return nil
}

// onRxComplete is called when CRS_DV falls (frame ended).
func (r *RMIITxRx) onRxComplete() {
	// Disable interrupt
	r.crsdvPin.SetInterrupt(0, nil)
	// Stop PIO
	r.smRx.SetEnabled(false)
	// Abort DMA and get bytes transferred
	n := r.RxBytesReceived()
	r.dmaRx.abort()
	if r.rxCallback != nil {
		r.rxCallback(n)
	}
}

// RxBytesReceived returns how many bytes have been received so far.
// Useful for polling during active receive.
func (r *RMIITxRx) RxBytesReceived() int {
	remaining := r.dmaRx.HW().TRANS_COUNT.Get()
	return len(r.rxBuf) - int(remaining)
}

// IsRxBusy returns true if a receive is in progress.
func (r *RMIITxRx) IsRxBusy() bool {
	return r.dmaRx.busy()
}

// Tx8 transmits data via DMA. Blocking until complete.
func (r *RMIITxRx) Tx8(data []byte) error {
	hw := r.dmaTx.HW()
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	hw.READ_ADDR.Set(uint32(uintptr(unsafe.Pointer(&data[0]))))
	hw.WRITE_ADDR.Set(uint32(uintptr(unsafe.Pointer(&r.smTx.TxReg().Reg))) + 3)
	hw.TRANS_COUNT.Set(uint32(len(data)))

	cc := r.dmaTx.CurrentConfig()
	cc.setTREQ_SEL(dmaPIO_TxDREQ(r.smTx))
	cc.setTransferDataSize(dmaTxSize8)
	cc.setChainTo(r.dmaTx.idx)
	cc.setReadIncrement(true)
	cc.setWriteIncrement(false)
	cc.setEnable(true)
	hw.CTRL_TRIG.Set(cc.CTRL)

	// Wait for DMA to complete
	for r.dmaTx.busy() {
		gosched()
	}

	// Wait for TX FIFO to drain
	for !r.smTx.IsTxFIFOEmpty() {
		gosched()
	}

	return nil
}

// SetTxEnabled enables or disables the TX state machine.
func (r *RMIITxRx) SetTxEnabled(enabled bool) {
	r.smTx.SetEnabled(enabled)
}
