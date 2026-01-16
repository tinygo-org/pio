package main

import (
	"bytes"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// See github.com/soypat/lneto/phy for a more complete
// and modular PHY/MAC/MII implementation.

// In order of level of abstraction, from lower level to higher level:
//   - mdio.go contains MDIO bus implementation and HAL definition.
//   - phy.go contains PHY access via MDIO. So phy, device, register address logic.
//   - rmii.go contains RMII integration with the PHY logic, so Rx/Tx added to MDIO.
//   - This file contains the main executable program which uses the logic shown.

// Pin configuration matching reference implementation
// Reference: https://github.com/sandeepmistry/pico-rmii-ethernet/blob/main/examples/httpd/main.c
const (
	// MDIO pins:
	pinMDIO = machine.GPIO0
	pinMDC  = machine.GPIO1
	// Reference clock: 		 (50MHz from PHY)
	// Mistakenly spelled as Retclk on breakout.
	pinRefClk = machine.GPIO2

	// RX pins: GPIO 3, 4, 5 (RXD0, RXD1, CRS_DV)
	pinCRSDV  = machine.GPIO3
	pinRxBase = machine.GPIO4

	// TX pins: GPIO 0, 1, 2 (TXD0, TXD1, TX_EN)
	pinTxBase = machine.GPIO6
)

// Our MAC address (locally administered).
var ourMAC = [6]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

// Broadcast MAC address.
var broadcastMAC = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

func main() {
	// Do initial test
	time.Sleep(2 * time.Second)
	println("start program")
	var mdio MDIOBitBang
	mdio.Configure(pinMDIO, pinMDC, 10_000, true)
	var addrs [32]uint8
	n, err := FindClause22PHYs(&mdio, addrs[:])
	if n >= 1 {
		println("found addrs:", addrs[0], "...")
	} else {
		println("no addrs")
		if err != nil {
			println("error:", err.Error())
		}
	}

	var rmii RMII
	err = rmii.Configure(RMIIConfig{
		PIO:       pio.PIO0,
		TxPinBase: pinTxBase,
		RxPinBase: pinRxBase,
		CRSDV:     pinCRSDV,
		RefClk:    pinRefClk,
		MDIOPin:   pinMDIO,
		MDCPin:    pinMDC,
		Baud:      10_000_000,
	})
	if err != nil {
		panic(err)
	}
	println("RMII configured")
	err = rmii.SetFirstAddr()
	if err != nil {
		panic(err)
	}
	ad := NewANAR().With10M()
	err = rmii.SetAdvertisement(ad)
	if err != nil {
		panic(err)
	}
	id1, _ := rmii.ID1()
	id2, _ := rmii.ID2()
	println("first addr set:", rmii.PHYAddr(), "id1,id2:", id1, id2)
	err = rmii.EnableAutoNegotiation(true)
	if err != nil {
		panic(err)
	}
	println("control enabled")

	// Wait for link to come up.
	println("waiting for link...")
	var linkMode LinkMode
	for {
		linkMode, err = rmii.NegotiatedLink()
		if err == nil && linkMode != LinkDown {
			break
		}
		print(".")
		time.Sleep(500 * time.Millisecond)
	}
	println("\nlink up:", linkMode.SpeedMbps(), "Mbps, full-duplex:", linkMode.IsFullDuplex())

	// Set up RX with callback.
	var rxBuf [1518]byte // Max Ethernet frame size
	var rcved bool
	rmii.rxtx.SetRxHandler(rxBuf[:], func(b []byte) {
		rcved = true
	})

	// Start receiving.
	err = rmii.rxtx.StartRx()
	if err != nil {
		panic(err)
	}
	println("RX started")

	// Main loop: send periodic packets.
	var txSeq uint32
	var lastTx time.Time
	for {
		// Send a broadcast packet every 5 seconds.
		if time.Since(lastTx) >= 5*time.Second {
			txSeq++
			frame := buildTestFrame(txSeq)
			err := rmii.rxtx.Tx8(frame)
			if err != nil {
				println("tx error:", err.Error())
			} else {
				println("tx: sent frame seq=", txSeq)
			}
			lastTx = time.Now()
		}
		if rcved {
			print("buf:")
			for i := range 32 {
				print(rxBuf[i], " ")
			}
			println("")
			n := ethernetFrameLength(rxBuf[:])
			parseAndPrintFrame(rxBuf[:n])
			rcved = false
			err = rmii.rxtx.StartRx()
			if err != nil {
				panic(err)
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// buildTestFrame builds a broadcast Ethernet frame with a test payload.
// EtherType 0x88B5 is used for local experimental use (IEEE 802 Local Experimental).
func buildTestFrame(seq uint32) []byte {
	const etherTypeExp = 0x88B5 // Local experimental EtherType
	payload := []byte("Hello from RP2040 RMII! Seq=0000")
	// Write sequence number into payload.
	payload[len(payload)-4] = byte(seq >> 24)
	payload[len(payload)-3] = byte(seq >> 16)
	payload[len(payload)-2] = byte(seq >> 8)
	payload[len(payload)-1] = byte(seq)

	// Build Ethernet frame: DstMAC(6) + SrcMAC(6) + EtherType(2) + Payload
	frame := make([]byte, 0, 14+len(payload))
	frame = append(frame, broadcastMAC[:]...)                             // Destination: broadcast
	frame = append(frame, ourMAC[:]...)                                   // Source: our MAC
	frame = append(frame, byte(etherTypeExp>>8), byte(etherTypeExp&0xFF)) // EtherType
	frame = append(frame, payload...)
	return frame
}

var zrx int
var buf [64]byte

// parseAndPrintFrame parses an Ethernet frame and prints info.
func parseAndPrintFrame(frame []byte) {
	if len(frame) < 14 {
		println("rx too small", len(frame))
		return // Too short for Ethernet header
	}
	var z [6]byte
	// Ethernet header: DstMAC(6) + SrcMAC(6) + EtherType(2)
	srcMAC := frame[6:12]
	if bytes.Equal(srcMAC, z[:]) {
		zrx++
		if zrx%100 == 0 {
			println("received zeroed frames zrx=", zrx)
		}
		return
	}
	etherType := uint16(frame[12])<<8 | uint16(frame[13])
	payloadLen := len(frame) - 14
	isIPv4 := etherType == 0x0800

	println("rx: src=", macString(srcMAC), "len=", payloadLen, "ipv4=", isIPv4)
}

// macString formats a MAC address as a string.
func macString(mac []byte) string {
	if len(mac) < 6 {
		return "?"
	}
	// Simple hex formatting without fmt package.
	const hex = "0123456789abcdef"
	var buf [17]byte
	for i := 0; i < 6; i++ {
		buf[i*3] = hex[mac[i]>>4]
		buf[i*3+1] = hex[mac[i]&0x0f]
		if i < 5 {
			buf[i*3+2] = ':'
		}
	}
	return string(buf[:])
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
