package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"machine"
	"time"

	"github.com/soypat/lneto/phy"
	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Pin configuration matching reference implementation.
// See makeEthernetMAC below to see how they are used.
const (
	MTU      = 1500
	MFU      = MTU + 14 + 4
	linkmode = phy.Link100FDX
	// MDIO pins:
	pinMDIO = machine.GPIO0
	pinMDC  = machine.GPIO1
	// Reference clock: 		 (50MHz from PHY)
	// Mistakenly spelled as Retclk on breakout.
	pinRefClk = machine.GPIO2

	// RX pins: GPIO 3, 4, 5 (RXD0, RXD1, CRS_DV)
	pinRxBase = machine.GPIO3

	// TX pins: GPIO 0, 1, 2 (TXD0, TXD1, TX_EN)
	pinTxBase = machine.GPIO7
)

type EthernetMAC struct {
	phy.Device
	rx piolib.RMIIRx
	tx piolib.RMIITx
}

// RMIIRx wrappers.

func (e *EthernetMAC) StartRx() error {
	return e.rx.StartRx()
}

func (e *EthernetMAC) StopRx() error {
	return e.rx.StopRx()
}

func (e *EthernetMAC) SetRxHandler(rxbuf []byte, callback func(buf []byte)) error {
	return e.rx.SetRxIRQHandler(rxbuf, callback)
}

func (e *EthernetMAC) ReceivedSinceStartRx() bool {
	return e.rx.ReceivedSinceStartRx()
}

func (e *EthernetMAC) InRx() bool {
	return e.rx.InRx()
}

// RMIITx wrappers.

func (e *EthernetMAC) SendFrame(frame []byte) error {
	return e.tx.SendFrame(frame)
}

func (e *EthernetMAC) IsSending() bool {
	return e.tx.IsSending()
}

// Our MAC address (locally administered).
var ourMAC = [6]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

// Broadcast MAC address.
var broadcastMAC = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

func main() {
	var err error
	time.Sleep(2 * time.Second)
	println("=== RMII Loopback Test ===")
	// var mdio MDIOBitBang
	// mdio.Configure(pinMDIO, pinMDC, 50_000, true)
	rmii, err := makeEthernetMAC()
	if err != nil {
		panic("creating ethernet MAC: " + err.Error())
	}
	id1, _ := rmii.ID1()
	id2, _ := rmii.ID2()
	bmsr, _ := rmii.BasicStatus()
	bmcr, _ := rmii.BasicControl()
	println("PHY addr:", rmii.PHYAddr(), "id:", id1, id2, "bmsr:", uintptr(bmsr), "bmcr:", uintptr(bmcr)) // uintptr forces hex print.
	err = rmii.SetLoopback(true)
	if err != nil {
		panic(err)
	}
	// Force 100Mbps full-duplex (no auto-neg needed for loopback cable)
	err = rmii.SetupForced(linkmode)
	if err != nil {
		panic(err)
	}
	println("forced linkmode ", linkmode.String())

	// Wait for link.
	println("waiting for link...")
	for i := 0; i < 20; i++ {
		up, _ := rmii.IsLinkUp()
		if up {
			println("link up!")
			break
		}
		print(".")
		time.Sleep(250 * time.Millisecond)
	}
	// Set up RX with callback.
	var rxBuf [MFU]byte
	var txbuf [MFU]byte
	var gotRx int
	err = rmii.SetRxHandler(rxBuf[:], func(b []byte) {
		gotRx = len(b)
	})
	if err != nil {
		panic("set rx handler: " + err.Error())
	}

	// Loopback test loop: send frame, wait for it to come back.
	var seq uint32
	for {
		for i := range rxBuf {
			rxBuf[i] = 0 // Ensure we are not reading previously stored data.
		}
		err := rmii.StartRx()
		if err != nil {
			panic("startrx: " + err.Error())
		}
		seq++
		n := putTestFrame(txbuf[:], seq)
		err = rmii.SendFrame(txbuf[:n])
		if err != nil {
			println("tx err:", err.Error())
		}

		// Wait for loopback (should come back quickly via cable)
		deadline := time.Now().Add(100 * time.Millisecond)
		for time.Now().Before(deadline) {
			if rmii.ReceivedSinceStartRx() {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if rmii.ReceivedSinceStartRx() {
			println("rx: got frame length ", gotRx)
			if bytes.Equal(rxBuf[:n], txbuf[:n]) {
				println("  MATCH: all bytes sent also received identically! length match:", gotRx == n)
			} else {
				print("  mismatch, first 20 bytes: ")
				for i := 0; i < 20; i++ {
					print(rxBuf[i], " ")
				}
				println()
			}
		} else {
			println("  no rx (timeout), plen=", n)
		}
		rmii.StopRx()
		time.Sleep(2 * time.Second)
	}
}

// buildTestFrame builds a broadcast Ethernet frame with a test payload.
// EtherType 0x88B5 is used for local experimental use (IEEE 802 Local Experimental).
func putTestFrame(dst []byte, seq uint32) int {
	const etherTypeExp = 0x88B5 // Local experimental EtherType
	copy(dst[:6], broadcastMAC[:6])
	copy(dst[6:12], ourMAC[:6])
	binary.BigEndian.PutUint16(dst[12:14], etherTypeExp)
	plen := copy(dst[14:], "s=")
	binary.BigEndian.PutUint32(dst[14+plen:], seq)
	return 14 + plen + 4
}

var zrx int
var buf [64]byte

// ethernetCRC32 calculates the IEEE 802.3 CRC-32 for Ethernet FCS.
func ethernetCRC32(data []byte) uint32 {
	const poly = 0xedb88320
	crc := uint32(0xffffffff)
	for _, b := range data {
		for bit := 0; bit < 8; bit++ {
			if (crc^uint32(b))&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
			b >>= 1
		}
	}
	return ^crc
}

// ethernetFrameLength scans data calculating CRC until it finds valid FCS.
// Returns frame length (excluding FCS) or 0 if no valid frame found.
// Inspired by Sandeep Mistry's pico-rmii-ethernet ethernet_frame_length().
func ethernetFrameLength(data []byte) int {
	const poly = 0xedb88320 // IEEE 802.3 CRC-32 polynomial (reversed)
	crc := uint32(0xffffffff)
	for i := 0; i < len(data)-4; i++ {
		b := data[i]
		for bit := 0; bit < 8; bit++ {
			if (crc^uint32(b))&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
			b >>= 1
		}
		// Check if next 4 bytes match inverted CRC (FCS)
		invCRC := ^crc
		if data[i+1] == byte(invCRC) &&
			data[i+2] == byte(invCRC>>8) &&
			data[i+3] == byte(invCRC>>16) &&
			data[i+4] == byte(invCRC>>24) {
			return i + 1 // Frame length excluding FCS
		}
	}
	return 0
}

func makeEthernetMAC() (*EthernetMAC, error) {
	mdiomsk := (1 << pinMDC) | (1 << pinMDIO)
	clkmsk := (1 << pinRefClk)
	txmsk := 0b111 << pinTxBase
	rxmsk := 0b111 << pinRxBase
	aliased := rxmsk & txmsk & mdiomsk & clkmsk
	if aliased != 0 {
		return nil, errors.New("aliased pins, check pin definitions")
	}
	var eth EthernetMAC
	// Setup PHY via MDIO.
	mdiobus := makeMDIOBus()
	var addrs [32]uint8
	_, err := phy.FindClause22PHYs(mdiobus, addrs[:])
	if err != nil {
		return nil, err
	}
	eth.Device.ConfigureAs22(mdiobus, addrs[0]) // Use first address found.
	PIO := pio.PIO0
	baud := 1e6 * linkmode.SpeedMbps()
	if baud > 100e6 || baud <= 0 {
		return nil, errors.New("unsupported link mode")
	}
	err = eth.rx.Configure(PIO, piolib.RMIIRxConfig{
		Baud:      uint32(baud),
		RxBase:    pinRxBase,
		IRQ:       0,
		IRQSource: pio.IRQS0,
	})
	if err != nil {
		return nil, err
	}
	err = eth.tx.Configure(PIO, piolib.RMIITxConfig{
		Baud:     uint32(baud),
		TxBuffer: make([]byte, MFU),
		TxBase:   pinTxBase,
	})
	if err != nil {
		return nil, err
	}
	return &eth, nil
}

func makeMDIOBus() phy.MDIOBus {
	var bus phy.MDIOBitBang
	const mdioDelay = 340 * time.Nanosecond // MDIO spec max turnaround time
	pinMDIO.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	pinMDC.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pinMDC.Low()
	bus.Configure(func(outBit bool) {
		// sendBit: set data, clock high, clock low
		if outBit {
			pinMDIO.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
		} else {
			pinMDIO.Low()
			pinMDIO.Configure(machine.PinConfig{Mode: machine.PinOutput})
		}
		time.Sleep(mdioDelay)
		pinMDC.High()
		time.Sleep(mdioDelay)
		pinMDC.Low()
	}, func() (inBit bool) {
		// getBit: clock high, read, clock low
		time.Sleep(mdioDelay)
		pinMDC.High()
		time.Sleep(mdioDelay)
		pinMDC.Low()
		return pinMDIO.Get()
	}, func(setOut bool) {
		// setDir: configure pin direction
		if setOut {
			pinMDIO.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
		} else {
			pinMDIO.Configure(machine.PinConfig{Mode: machine.PinInput})
		}
	})
	return &bus
}
