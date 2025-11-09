//go:build rp2040 || rp2350

package piolib

import (
	"errors"
	"machine"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// RMIITxRx is the Reduced Media Independent Interface for 100Mbps Ethernet PHY communication.
// It uses two state machines: one for TX and one for RX.
// Inspired by Sandeep Mistry's implementation at https://github.com/sandeepmistry/pico-rmii-ethernet/tree/main/src
type RMIITxRx struct {
	smTx         pio.StateMachine
	smRx         pio.StateMachine
	programOffTx uint8
	programOffRx uint8
	dmaTx        dmaChannel
	dmaRx        dmaChannel
}

// RMIITxRxConfig configures the RMII interface pins and parameters.
type RMIITxRxConfig struct {
	Baud uint32
	// TxPin is the base pin for RMII TX (TXD0, TXD1, TX_EN).
	// Requires 3 consecutive pins.
	TxPin machine.Pin
	// RxPin is the base pin for RMII RX (RXD0, RXD1).
	// Requires 2 consecutive pins.
	RxPin machine.Pin
	// CRSDVPin is the Carrier Sense/Data Valid pin (also called RX_DV).
	CRSDVPin machine.Pin
	// RefClkPin is the 50MHz reference clock input from PHY.
	RefClkPin machine.Pin
}

// NewRMIITxRx creates a new RMII interface using two state machines (TX and RX).
// The TX and RX state machines should be from the same PIO block.
func NewRMIITxRx(smTx, smRx pio.StateMachine, cfg RMIITxRxConfig) (*RMIITxRx, error) {
	if smTx.PIO().BlockIndex() != smRx.PIO().BlockIndex() {
		return nil, errors.New("TX and RX state machines must be from the same PIO block")
	}

	// Claim state machines
	smTx.TryClaim()
	smRx.TryClaim()

	Pio := smTx.PIO()
	var asm pio.AssemblerV0

	// RX Program: Wait for sync pattern, then continuously read 2 bits
	// .program rmii_ethernet_phy_rx_data
	//     wait 0 pin 2    ; Wait for CRSDV low
	//     wait 0 pin 0    ; Wait for RXD0 low
	//     wait 0 pin 1    ; Wait for RXD1 low
	//     wait 1 pin 2    ; Wait for CRSDV high
	//     wait 1 pin 0    ; Wait for RXD0 high
	//     wait 1 pin 1    ; Wait for RXD1 high
	// .wrap_target
	//     in pins, 2      ; Continuously read 2 bits
	// .wrap
	// if program changes wrap target must be changed manually.
	// Worry not, will fail to compile if wrap target is invalid.
	const rxWrapTarget = 6
	rxProgram := [7]uint16{
		asm.WaitPin(false, 2).Encode(), // wait 0 pin 2 (CRSDV)
		asm.WaitPin(false, 0).Encode(), // wait 0 pin 0 (RXD0)
		asm.WaitPin(false, 1).Encode(), // wait 0 pin 1 (RXD1)
		asm.WaitPin(true, 2).Encode(),  // wait 1 pin 2 (CRSDV)
		asm.WaitPin(true, 0).Encode(),  // wait 1 pin 0 (RXD0)
		asm.WaitPin(true, 1).Encode(),  // wait 1 pin 1 (RXD1)
		rxWrapTarget:// .wrap_target Once all pins have waited we receive all data in loop.
		asm.In(pio.InSrcPins, 2).Encode(), // in pins, 2
	}

	// Add RX program first (longer, more likely to fail if PIO memory full)
	rxOffset, err := Pio.AddProgram(rxProgram[:], -1)
	if err != nil {
		return nil, err
	}

	// TX Program: Simple output of 3 pins (TXD0, TXD1, TX_EN)
	// .program rmii_ethernet_phy_tx_data
	// .wrap_target
	//     out pins, 3
	// .wrap
	txProgram := [1]uint16{
		asm.Out(pio.OutDestPins, 3).Encode(), // out pins, 3
	}

	txOffset, err := Pio.AddProgram(txProgram[:], -1)
	if err != nil {
		Pio.ClearProgramSection(rxOffset, uint8(len(rxProgram)))
		return nil, err
	}

	// Configure RX state machine
	rxcfg := pio.DefaultStateMachineConfig()
	rxcfg.SetWrap(rxOffset+rxWrapTarget, rxOffset+uint8(len(rxProgram))-1)
	rxcfg.SetInPins(cfg.RxPin, 2) // RXD0, RXD1
	// Shift left, autopush enabled, push threshold 32 bits
	rxcfg.SetInShift(false, true, 32)
	rxcfg.SetClkDivIntFrac(10, 0)
	rxcfg.SetFIFOJoin(pio.FifoJoinRx)

	// Configure TX state machine
	txcfg := pio.DefaultStateMachineConfig()
	txcfg.SetWrap(txOffset, txOffset+uint8(len(txProgram))-1)
	txcfg.SetOutPins(cfg.TxPin, 3)
	// Shift right, autopull enabled, pull threshold 32 bits
	txcfg.SetOutShift(true, true, 32)
	// Clock divider: run at 50MHz / 4 = 12.5MHz for RMII (2 bits per clock at 50MHz = 4 clocks per 2-bit dibit)
	// Reference implementation uses divider of 10 to get effective rate
	txcfg.SetClkDivIntFrac(10, 0)
	txcfg.SetFIFOJoin(pio.FifoJoinTx)

	// Configure pins
	pinCfg := machine.PinConfig{Mode: Pio.PinMode()}

	// Configure TX pins (TXD0, TXD1, TX_EN)
	for i := 0; i < 3; i++ {
		pin := cfg.TxPin + machine.Pin(i)
		pin.Configure(pinCfg)
	}

	// Configure RX pins (RXD0, RXD1, CRSDV)
	cfg.RxPin.Configure(pinCfg)
	(cfg.RxPin + 1).Configure(pinCfg)
	cfg.CRSDVPin.Configure(pinCfg)

	// RefClk is input from PHY, configure as input
	cfg.RefClkPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	// Set TX pins as output, initially low
	txPinMask := uint32(0b111 << cfg.TxPin)
	smTx.SetPindirsMasked(txPinMask, txPinMask)
	smTx.SetPinsMasked(0, txPinMask)

	// Set RX pins as input
	rxPinMask := uint32(0b11<<cfg.RxPin | 1<<cfg.CRSDVPin)
	smRx.SetPindirsMasked(0, rxPinMask)

	// Initialize state machines
	smRx.Init(rxOffset, rxcfg)
	smTx.Init(txOffset, txcfg)

	smTx.SetEnabled(true)
	smRx.SetEnabled(true)
	return &RMIITxRx{
		smTx:         smTx,
		smRx:         smRx,
		programOffTx: txOffset,
		programOffRx: rxOffset,
	}, nil
}

// IsEnabled returns true if both TX and RX state machines are enabled.
func (r *RMIITxRx) IsEnabled() bool {
	return r.smTx.IsEnabled() && r.smRx.IsEnabled()
}

// SetEnabled enables or disables both TX and RX state machines.
func (r *RMIITxRx) SetEnabled(enabled bool) {
	r.smTx.SetEnabled(enabled)
	r.smRx.SetEnabled(enabled)
}

// SetTxEnabled enables or disables only the TX state machine.
func (r *RMIITxRx) SetTxEnabled(enabled bool) {
	r.smTx.SetEnabled(enabled)
}

// SetRxEnabled enables or disables only the RX state machine.
func (r *RMIITxRx) SetRxEnabled(enabled bool) {
	r.smRx.SetEnabled(enabled)
}

// Tx8 transmits a byte buffer to the PHY via the TX state machine.
// Data is sent as 2-bit dibits with TX_EN asserted.
func (r *RMIITxRx) Tx8(data []byte) error {
	if r.IsTxDMAEnabled() {
		return r.tx8DMA(data)
	}
	return r.tx8(data)
}

func (r *RMIITxRx) tx8(data []byte) error {
	deadline := r.dmaTx.dl.newDeadline()
	for _, b := range data {
		for r.smTx.IsTxFIFOFull() {
			if deadline.expired() {
				return errTimeout
			}
			gosched()
		}
		r.smTx.TxPut(uint32(b))
	}
	// Wait for TX FIFO to drain
	for !r.smTx.IsTxFIFOEmpty() {
		if deadline.expired() {
			return errTimeout
		}
		gosched()
	}
	return nil
}

func (r *RMIITxRx) tx8DMA(data []byte) error {
	dreq := dmaPIO_TxDREQ(r.smTx)
	err := r.dmaTx.Push8((*byte)(unsafe.Pointer(&r.smTx.TxReg().Reg)), data, dreq)
	if err != nil {
		return err
	}
	// Wait for TX FIFO to drain
	deadline := r.dmaTx.dl.newDeadline()
	for !r.smTx.IsTxFIFOEmpty() {
		if deadline.expired() {
			return errTimeout
		}
		gosched()
	}
	return nil
}

// Rx8 receives a byte buffer from the PHY via the RX state machine.
// Data is received as 2-bit dibits.
func (r *RMIITxRx) Rx8(data []byte) error {
	if r.IsRxDMAEnabled() {
		return r.rx8DMA(data)
	}
	return r.rx8(data)
}

func (r *RMIITxRx) rx8(data []byte) error {
	deadline := r.dmaRx.dl.newDeadline()
	for i := range data {
		for r.smRx.IsRxFIFOEmpty() {
			if deadline.expired() {
				return errTimeout
			}
			gosched()
		}
		data[i] = byte(r.smRx.RxGet())
	}
	return nil
}

func (r *RMIITxRx) rx8DMA(data []byte) error {
	dreq := dmaPIO_RxDREQ(r.smRx)
	err := r.dmaRx.Pull8(data, (*byte)(unsafe.Pointer(&r.smRx.RxReg().Reg)), dreq)
	if err != nil {
		return err
	}
	return nil
}

// IsTxDMAEnabled returns true if TX DMA is enabled.
func (r *RMIITxRx) IsTxDMAEnabled() bool {
	return r.dmaTx.helperIsEnabled()
}

// IsRxDMAEnabled returns true if RX DMA is enabled.
func (r *RMIITxRx) IsRxDMAEnabled() bool {
	return r.dmaRx.helperIsEnabled()
}

// EnableTxDMA enables or disables DMA for TX operations.
func (r *RMIITxRx) EnableTxDMA(enabled bool) error {
	return r.dmaTx.helperEnableDMA(enabled)
}

// EnableRxDMA enables or disables DMA for RX operations.
func (r *RMIITxRx) EnableRxDMA(enabled bool) error {
	return r.dmaRx.helperEnableDMA(enabled)
}

// SetTimeout sets the read/write timeout for both TX and RX operations.
// Use 0 as argument to disable timeouts.
func (r *RMIITxRx) SetTimeout(timeout time.Duration) {
	r.dmaTx.dl.setTimeout(timeout)
	r.dmaRx.dl.setTimeout(timeout)
}

// ClearTxFIFO clears the TX FIFO.
func (r *RMIITxRx) ClearTxFIFO() {
	r.smTx.ClearFIFOs()
}

// ClearRxFIFO clears the RX FIFO.
func (r *RMIITxRx) ClearRxFIFO() {
	r.smRx.ClearFIFOs()
}
