package main

import (
	"bytes"
	"encoding/binary"
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
	pinRxBase = machine.GPIO3
	pinCRSDV  = machine.GPIO5

	// TX pins: GPIO 0, 1, 2 (TXD0, TXD1, TX_EN)
	pinTxBase = machine.GPIO6
)

// Our MAC address (locally administered).
var ourMAC = [6]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}

// Broadcast MAC address.
var broadcastMAC = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

func main() {
	time.Sleep(2 * time.Second)
	println("=== RMII Loopback Test ===")

	var rmii RMII
	err := rmii.Configure(RMIIConfig{
		PIO:       pio.PIO0,
		TxPinBase: pinTxBase,
		RxPinBase: pinRxBase,
		CRSDV:     pinCRSDV,
		RefClk:    pinRefClk,
		MDIOPin:   pinMDIO,
		MDCPin:    pinMDC,
		Baud:      100_000_000,
	})
	if err != nil {
		panic(err)
	}

	id1, _ := rmii.ID1()
	id2, _ := rmii.ID2()
	println("PHY addr:", rmii.PHYAddr(), "id:", id1, id2)

	// Force 100Mbps full-duplex (no auto-neg needed for loopback cable)
	err = rmii.SetupForced(Link100FDX)
	if err != nil {
		panic(err)
	}
	println("forced 100M-FDX")

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
	var rxBuf [1518]byte // Max Ethernet frame size
	var gotRx bool
	rmii.rxtx.SetRxHandler(rxBuf[:], func(b []byte) {
		gotRx = true
	})

	// Start receiving.
	err = rmii.rxtx.StartRx()
	if err != nil {
		panic(err)
	}
	println("RX started")

	// Loopback test loop: send frame, wait for it to come back.
	var seq uint32
	for {
		seq++
		txFrame := buildTestFrame(seq)

		err := rmii.rxtx.Tx8(txFrame)
		if err != nil {
			println("tx err:", err.Error())
		} else {
			println("tx seq=", seq, "len=", len(txFrame))
		}

		// Wait for loopback (should come back quickly via cable)
		deadline := time.Now().Add(100 * time.Millisecond)
		for time.Now().Before(deadline) {
			if gotRx {
				break
			}
			time.Sleep(time.Millisecond)
		}

		if gotRx {
			println("rx: got frame")
			if bytes.Equal(rxBuf[:len(txFrame)], txFrame) {
				println("  MATCH!")
			} else {
				print("  mismatch, first 20 bytes: ")
				for i := 0; i < 20; i++ {
					print(rxBuf[i], " ")
				}
				println()
			}
			gotRx = false
			rmii.rxtx.StartRx()
		} else {
			println("  no rx (timeout)")
		}

		time.Sleep(2 * time.Second)
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

	// Build Ethernet frame: DstMAC(6) + SrcMAC(6) + EtherType(2) + Payload + FCS(4)
	frame := make([]byte, 0, 14+len(payload)+4)
	frame = append(frame, broadcastMAC[:]...)                             // Destination: broadcast
	frame = append(frame, ourMAC[:]...)                                   // Source: our MAC
	frame = append(frame, byte(etherTypeExp>>8), byte(etherTypeExp&0xFF)) // EtherType
	frame = append(frame, payload...)
	return appendFCS(frame) // Add 4-byte FCS
}

var zrx int
var buf [64]byte

// parseAndPrintFrame parses an Ethernet frame and prints info.
func parseAndPrintFrame(frame []byte) {
	var z [6]byte
	// Ethernet header: DstMAC(6) + SrcMAC(6) + EtherType(2)
	dstMAC := frame[0:6]
	srcMAC := frame[6:12]
	if bytes.Equal(srcMAC, z[:]) && bytes.Equal(dstMAC, z[:]) {
		zrx++
		if zrx%100 == 0 {
			println("received zeroed frames zrx=", zrx)
		}
		return
	}
	etherType := binary.BigEndian.Uint16(frame[12:])
	payloadLen := len(frame) - 14
	isIPv4 := etherType == 0x0800
	println("\trx: src=", macString(srcMAC), "len=", payloadLen, "ethertype=", uintptr(etherType), "ipv4=", isIPv4)

	// Print IPv4 addresses if this is an IPv4 frame.
	// IPv4 header: starts at byte 14, src IP at offset 12, dst IP at offset 16.
	if isIPv4 && len(frame) >= 34 { // 14 (eth) + 20 (min IPv4 header)
		srcIP := frame[26:30] // Ethernet(14) + IPv4 src offset(12)
		dstIP := frame[30:34] // Ethernet(14) + IPv4 dst offset(16)
		println("\tipv4: src=", ipString(srcIP), "dst=", ipString(dstIP))
	}
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

// ipString formats an IPv4 address as a dotted decimal string.
func ipString(ip []byte) string {
	if len(ip) < 4 {
		return "?"
	}
	// Simple decimal formatting without fmt package.
	var buf [15]byte // max "255.255.255.255"
	n := 0
	for i := 0; i < 4; i++ {
		if i > 0 {
			buf[n] = '.'
			n++
		}
		n += putUint8(buf[n:], ip[i])
	}
	return string(buf[:n])
}

// putUint8 writes a uint8 as decimal digits and returns bytes written.
func putUint8(buf []byte, v uint8) int {
	if v >= 100 {
		buf[0] = '0' + v/100
		buf[1] = '0' + (v/10)%10
		buf[2] = '0' + v%10
		return 3
	} else if v >= 10 {
		buf[0] = '0' + v/10
		buf[1] = '0' + v%10
		return 2
	}
	buf[0] = '0' + v
	return 1
}

// appendFCS calculates and appends the 4-byte Ethernet FCS (CRC-32) to the frame.
func appendFCS(frame []byte) []byte {
	crc := ethernetCRC32(frame)
	// Append FCS in little-endian order (inverted CRC)
	return append(frame, byte(crc), byte(crc>>8), byte(crc>>16), byte(crc>>24))
}

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
