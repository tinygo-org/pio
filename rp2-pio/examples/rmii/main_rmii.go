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
	id1, _ := rmii.ID1()
	id2, _ := rmii.ID2()
	println("first addr set:", rmii.PHYAddr(), "id1,id2:", id1, id2)
	err = rmii.SetControlEnable(true)
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

	// Enable RMII Tx/Rx.
	rmii.rxtx.SetEnabled(true)

	// Main loop: send periodic packets and receive.
	var rxBuf [1518]byte // Max Ethernet frame size
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

		// Try to receive a packet (non-blocking check).
		err := rmii.rxtx.Rx8(rxBuf[:])
		if err == nil {
			// Parse and print received frame.
			parseAndPrintFrame(rxBuf[:])
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

// parseAndPrintFrame parses an Ethernet frame and prints info.
func parseAndPrintFrame(frame []byte) {
	if len(frame) < 14 {
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
