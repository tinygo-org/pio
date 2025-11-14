//go:build rp2040 || rp2350

package piolib

import (
	"errors"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// RMII provides a complete Reduced Media Independent Interface implementation
// with MDIO/MDC management interface for PHY register access.
// Inspired by Sandeep Mistry's implementation at https://github.com/sandeepmistry/pico-rmii-ethernet
type RMII struct {
	rxtx     RMIITxRx
	zmdio    bool
	mdio     machine.Pin
	mdc      machine.Pin
	phyAddr  uint8
	rxDVPin  machine.Pin
	rxBuffer []byte
	txBuffer []byte
}

// RMIIConfig configures the complete RMII interface including MDIO/MDC pins.
type RMIIConfig struct {
	// TxRx contains the configuration for the PIO-based TX/RX interface
	TxRx RMIITxRxConfig
	// MDIO is the Management Data Input/Output pin for PHY register access
	MDIO machine.Pin
	// MDC is the Management Data Clock pin
	MDC machine.Pin
	// RxBufferSize is the size of the receive buffer (default 2048 if 0)
	RxBufferSize int
	// TxBufferSize is the size of the transmit buffer (default 2048 if 0)
	TxBufferSize int
	// NoZMDIO avoids using high impedance Z level for HIGH pin state on MDIO as stated by RMII specification.
	NoZMDIO bool
}

// NewRMII creates a new complete RMII interface with MDIO/MDC management.
func NewRMII(smTx, smRx pio.StateMachine, cfg RMIIConfig) (*RMII, error) {
	// Create the low-level TX/RX interface
	rxtx, err := NewRMIITxRx(smTx, smRx, cfg.TxRx)
	if err != nil {
		return nil, err
	}

	// Set default buffer sizes
	rxBufSize := cfg.RxBufferSize
	if rxBufSize == 0 {
		rxBufSize = 2048
	}
	txBufSize := cfg.TxBufferSize
	if txBufSize == 0 {
		txBufSize = 2048
	}

	rmii := &RMII{
		rxtx:     *rxtx,
		mdio:     cfg.MDIO,
		mdc:      cfg.MDC,
		rxDVPin:  cfg.TxRx.CRSDVPin,
		rxBuffer: make([]byte, rxBufSize),
		txBuffer: make([]byte, txBufSize),
		zmdio:    !cfg.NoZMDIO,
	}

	// Configure MDIO/MDC pins
	// rmii.mdCfg()
	return rmii, nil
}

// // DiscoverPHY scans MDIO addresses 0-31 to find a connected PHY.
// // Returns the PHY address or an error if no PHY is found.
// func (r *RMII) DiscoverPHY() error {
// 	for addr := uint8(0); addr < 32; addr++ {
// 		val, err := r.MDIORead(addr, 0)
// 		if err != nil {
// 			continue
// 		}
// 		if val != 0xffff && val != 0x0000 {
// 			r.phyAddr = addr
// 			return nil
// 		}
// 	}
// 	return errors.New("no PHY found on MDIO bus")
// }

// InitPHY initializes the PHY with auto-negotiation settings.
// Must be called after DiscoverPHY().
// Reference: netif_rmii_ethernet_low_init() from rmii_ethernet.c
func (r *RMII) InitPHY() error {
	// Write to register 4 (Advertisement): 0x61
	// Configure advertised capabilities (10/100 Mbps support)
	if err := r.MDIOWrite(r.phyAddr, 4, 0x61); err != nil {
		return err
	}

	// Write to register 0 (Control): 0x1000
	// Enable auto-negotiation (bit 12)
	if err := r.MDIOWrite(r.phyAddr, 0, 0x1000); err != nil {
		return err
	}

	return nil
}

// PHYAddr returns the discovered PHY address. Set after [RMII.DiscoverPHY] success.
func (r *RMII) PHYAddr() uint8 {
	return r.phyAddr
}

// MDIO low-level clock operations
// Reference: netif_rmii_ethernet_mdio_clock_out() and netif_rmii_ethernet_mdio_clock_in()
// from rmii_ethernet.c

// mdioClockOut outputs a bit on MDIO while pulsing MDC clock.
func (r *RMII) mdioClockOut(bit bool) {
	r.mdioSet(bit)
	time.Sleep(time.Microsecond)
	r.mdc.High()
	time.Sleep(time.Microsecond)
	r.mdc.Low()
	time.Sleep(time.Microsecond)
}

func (r *RMII) mdioSet(b bool) {
	if r.zmdio {
		if b {
			r.mdioZHigh()
		} else {
			r.mdioLow()
		}
	} else {
		r.mdio.Set(b)
	}
}

func (r *RMII) mdioZHigh() {
	// RMII z pin level means high impedance, pull up resistor.
	r.mdio.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
}

func (r *RMII) mdioLow() {
	// RMII 0 pin level sets as output
	r.mdio.Low()
	r.mdio.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

// mdioClockIn reads a bit from MDIO while pulsing MDC clock.
func (r *RMII) mdioClockIn() bool {
	r.mdc.High()
	time.Sleep(time.Microsecond)
	bit := r.mdio.Get()
	time.Sleep(time.Microsecond)
	r.mdc.Low()
	time.Sleep(time.Microsecond)
	return bit
}

func (r *RMII) mdCfg() {
	r.mdc.Low()
	r.mdio.Configure(machine.PinConfig{Mode: machine.PinOutput})
	r.mdc.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

// MDIORead reads a 16-bit register from the PHY via MDIO.
// Implements IEEE 802.3 MDIO frame format for read operation.
// Reference: netif_rmii_ethernet_mdio_read() from rmii_ethernet.c
func (r *RMII) MDIORead(phyAddr uint8, regAddr uint8) (uint16, error) {
	if phyAddr > 31 || regAddr > 31 {
		return 0, errors.New("MDIO address out of range")
	}
	r.mdCfg()

	// Preamble: 32 bits of '1'
	for i := 0; i < 32; i++ {
		r.mdioClockOut(true)
	}

	// Start of frame: 01
	r.mdioClockOut(false)
	r.mdioClockOut(true)

	// Opcode: 10 (read)
	r.mdioClockOut(true)
	r.mdioClockOut(false)

	// PHY address: 5 bits MSB first
	for i := 4; i >= 0; i-- {
		r.mdioClockOut((phyAddr>>uint(i))&0x01 != 0)
	}

	// Register address: 5 bits MSB first
	for i := 4; i >= 0; i-- {
		r.mdioClockOut((regAddr>>uint(i))&0x01 != 0)
	}

	// Turnaround: switch MDIO to input, read 2 bits (should be Z0)
	r.mdio.Configure(machine.PinConfig{Mode: machine.PinInput})
	r.mdioClockOut(false) // Z bit
	r.mdioClockOut(false) // 0 bit

	// Read 16 data bits MSB first
	var data uint16
	for i := 15; i >= 0; i-- {
		data <<= 1
		data |= uint16(b2u8(r.mdioClockIn()))
	}

	return data, nil
}

// MDIOWrite writes a 16-bit value to a PHY register via MDIO.
// Implements IEEE 802.3 MDIO frame format for write operation.
// Reference: netif_rmii_ethernet_mdio_write() from rmii_ethernet.c
func (r *RMII) MDIOWrite(phyAddr uint8, regAddr uint8, value uint16) error {
	if phyAddr > 31 || regAddr > 31 {
		return errors.New("MDIO address out of range")
	}
	r.mdCfg()

	// Preamble: 32 bits of '1'
	for i := 0; i < 32; i++ {
		r.mdioClockOut(true)
	}

	// Start of frame: 01
	r.mdioClockOut(false)
	r.mdioClockOut(true)

	// Opcode: 01 (write)
	r.mdioClockOut(false)
	r.mdioClockOut(true)

	// PHY address: 5 bits MSB first
	for i := 4; i >= 0; i-- {
		r.mdioClockOut((phyAddr>>uint(i))&0x01 != 0)
	}

	// Register address: 5 bits MSB first
	for i := 4; i >= 0; i-- {
		r.mdioClockOut((regAddr>>uint(i))&0x01 != 0)
	}

	// Turnaround: 10
	r.mdioClockOut(true)
	r.mdioClockOut(false)

	// Write 16 data bits MSB first
	for i := 15; i >= 0; i-- {
		r.mdioClockOut((value>>uint(i))&0x01 != 0)
	}

	// Release MDIO bus to high impedance
	r.mdio.Configure(machine.PinConfig{Mode: machine.PinInput})

	return nil
}

// CRC32 computes the Ethernet CRC32 for the given data.
// Uses the polynomial 0xedb88320 (reversed representation).
// Reference: netif_rmii_ethernet_crc() from rmii_ethernet.c
func (r *RMII) CRC32(data []byte) uint32 {
	const polynomial = 0xedb88320
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc ^= uint32(b)
		for bit := 0; bit < 8; bit++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ polynomial
			} else {
				crc = crc >> 1
			}
		}
	}
	return ^crc
}

// Pass-through methods to underlying rxtx

// SetEnabled enables or disables both TX and RX state machines.
func (r *RMII) SetEnabled(enabled bool) {
	r.rxtx.SetEnabled(enabled)
}

func (r *RMII) EnableDMA(enabled bool) error {
	err := r.rxtx.EnableRxDMA(enabled)
	if err != nil {
		return err
	}
	err = r.rxtx.EnableTxDMA(enabled)
	return err
}

// SetTimeout sets the read/write timeout for both TX and RX operations.
func (r *RMII) SetTimeout(timeout time.Duration) {
	r.rxtx.SetTimeout(timeout)
}

// RxTx returns a reference to the underlying RMIItxrx for low-level access.
func (r *RMII) RxTx() *RMIITxRx {
	return &r.rxtx
}

// Frame transmission and reception
// Reference: netif_rmii_ethernet_output() from rmii_ethernet.c

// TxFrame transmits an Ethernet frame with preamble, SFD, data, and CRC.
// The data should be the complete Ethernet frame (destination MAC, source MAC, type, payload).
// Minimum frame size is 60 bytes (excluding preamble/SFD/CRC).
func (r *RMII) TxFrame(frame []byte) error {
	if len(frame) < 60 {
		return errors.New("frame too small (minimum 60 bytes)")
	}
	if len(frame) > 1518 {
		return errors.New("frame too large (maximum 1518 bytes)")
	}

	// Compute CRC32
	crc := r.CRC32(frame)

	// Encode frame: preamble + SFD + data + CRC + IPG
	// Each byte is encoded as 4 nibbles with TX_EN asserted
	const preambleNibbles = 31
	const sfdNibbles = 1
	dataAndCrcLen := len(frame) + 4 // frame + 4 byte CRC
	const ipgNibbles = 12 * 4       // 12 bytes * 4 nibbles per byte = 48

	totalNibbles := preambleNibbles + sfdNibbles + (dataAndCrcLen * 4) + ipgNibbles

	// Ensure we don't overflow the tx buffer
	if totalNibbles > len(r.txBuffer) {
		return errors.New("frame too large for TX buffer")
	}

	idx := 0

	// Preamble: 31 × 0x05 (alternating 01 pattern with TX_EN)
	for i := 0; i < preambleNibbles; i++ {
		r.txBuffer[idx] = 0x05
		idx++
	}

	// SFD: 1 × 0x07 (10101011 start frame delimiter)
	r.txBuffer[idx] = 0x07
	idx++

	// Encode frame data: each byte as 4 nibbles with 0x04 prefix (TX_EN bit)
	for _, b := range frame {
		r.txBuffer[idx] = 0x04 | ((b >> 0) & 0x03) // bits [1:0]
		idx++
		r.txBuffer[idx] = 0x04 | ((b >> 2) & 0x03) // bits [3:2]
		idx++
		r.txBuffer[idx] = 0x04 | ((b >> 4) & 0x03) // bits [5:4]
		idx++
		r.txBuffer[idx] = 0x04 | ((b >> 6) & 0x03) // bits [7:6]
		idx++
	}

	// Encode CRC: 4 bytes as nibbles
	for i := 0; i < 4; i++ {
		crcByte := byte(crc >> uint(i*8))
		r.txBuffer[idx] = 0x04 | ((crcByte >> 0) & 0x03)
		idx++
		r.txBuffer[idx] = 0x04 | ((crcByte >> 2) & 0x03)
		idx++
		r.txBuffer[idx] = 0x04 | ((crcByte >> 4) & 0x03)
		idx++
		r.txBuffer[idx] = 0x04 | ((crcByte >> 6) & 0x03)
		idx++
	}

	// Inter-packet gap: 12 × 0x00 (idle, TX_EN low)
	for i := 0; i < ipgNibbles; i++ {
		r.txBuffer[idx] = 0x00
		idx++
	}

	// Transmit via the underlying rxtx
	return r.rxtx.Tx8(r.txBuffer[:idx])
}

// EnableRxInterrupt enables GPIO interrupt on RX_DV falling edge for frame detection.
// The callback will be invoked when a frame reception completes (RX_DV goes low).
// Reference: netif_rmii_ethernet_rx_dv_falling_callback() from rmii_ethernet.c
func (r *RMII) EnableRxInterrupt(callback func(machine.Pin)) error {
	if callback == nil {
		return errors.New("callback cannot be nil")
	}
	return r.rxDVPin.SetInterrupt(machine.PinFalling, callback)
}

// DisableRxInterrupt disables the RX_DV falling edge interrupt.
func (r *RMII) DisableRxInterrupt() error {
	return r.rxDVPin.SetInterrupt(machine.PinFalling, nil)
}

// OnRxComplete is called when RX_DV falling edge is detected.
// This stops the RX state machine and aborts DMA, signaling frame completion.
// Users should call this from their interrupt handler.
// Reference: netif_rmii_ethernet_rx_dv_falling_callback() from rmii_ethernet.c
func (r *RMII) OnRxComplete() {
	r.rxtx.smRx.SetEnabled(false)
	// Note: DMA abort is internal to dmaChannel, happens automatically when disabled
}

// StartRxDMA starts continuous DMA reception into the internal buffer.
// This should be combined with interrupt handling on RX_DV for frame detection.
// Call EnableRxInterrupt() with a callback that invokes OnRxComplete().
func (r *RMII) StartRxDMA() error {
	r.rxtx.smRx.SetEnabled(true)
	return r.rxtx.Rx8(r.rxBuffer)
}

// RxBuffer returns a reference to the internal RX buffer for direct access.
// Useful for interrupt-driven reception where you need to inspect the buffer.
func (r *RMII) RxBuffer() []byte {
	return r.rxBuffer
}

// TxBuffer returns a reference to the internal TX buffer for direct access.
func (r *RMII) TxBuffer() []byte {
	return r.txBuffer
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
