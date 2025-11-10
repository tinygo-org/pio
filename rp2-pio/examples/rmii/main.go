package main

import (
	"machine"
	"strconv"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Pin configuration matching reference implementation
// Reference: https://github.com/sandeepmistry/pico-rmii-ethernet/blob/main/examples/httpd/main.c
const (
	// TX pins: GPIO 0, 1, 2 (TXD0, TXD1, TX_EN)
	pinTxBase = machine.GPIO0

	// RX pins: GPIO 3, 4, 5 (RXD0, RXD1, CRS_DV)
	pinRxBase = machine.GPIO3
	pinCRSDV  = machine.GPIO5

	// MDIO pins:
	pinMDC  = machine.GPIO6
	pinMDIO = machine.GPIO7

	// Reference clock: 		 (50MHz from PHY)
	pinRefClk = machine.GPIO6
)

// Network configuration
var (
	// MAC address (locally administered)
	macAddr = [6]byte{0x02, 0x00, 0x00, 0x12, 0x34, 0x56}

	// Static IP configuration (before DHCP)
	ipAddr  = [4]byte{192, 168, 1, 100}
	netmask = [4]byte{255, 255, 255, 0}
	gateway = [4]byte{192, 168, 1, 1}
)

func main() {
	// Sleep to allow serial monitor to connect
	time.Sleep(2 * time.Second)
	println("RMII Ethernet HTTP Server")
	println("========================")

	// Initialize RMII interface
	rmii, err := initRMII(pio.PIO0)
	if err != nil {
		panic("Failed to initialize RMII: " + err.Error())
	}

	// Discover and initialize PHY
	println("\nDiscovering PHY...")
	if err := rmii.DiscoverPHY(); err != nil {
		panic("PHY discovery failed: " + err.Error())
	}
	println("PHY found at address:", rmii.PHYAddr())

	// Initialize PHY with auto-negotiation
	println("Initializing PHY...")
	if err := rmii.InitPHY(); err != nil {
		panic("PHY initialization failed: " + err.Error())
	}

	// Wait for link to come up
	println("Waiting for link...")
	waitForLink(rmii)
	println("Link is UP!")

	// Print network configuration
	println("\nNetwork Configuration:")
	println("  MAC:", string(appendHexSep(nil, macAddr[:], ':')))
	println("  IP:", string(appendDecSep(nil, ipAddr[:], '.')))
	println("  Netmask:", string(appendDecSep(nil, netmask[:], '.')))
	println("  Gateway:", string(appendDecSep(nil, gateway[:], '.')))
	println("\nHTTP server listening on port 80")

	// Enable RX DMA with interrupt handling
	err = rmii.EnableDMA(true)
	if err != nil {
		panic("failed to enabled DMA:" + err.Error())
	}

	// Set up RX interrupt for frame detection
	if err := rmii.EnableRxInterrupt(func(pin machine.Pin) {
		rmii.OnRxComplete()
	}); err != nil {
		panic("Failed to enable RX interrupt: " + err.Error())
	}

	// Start RX DMA
	go func() {
		for {
			if err := rmii.StartRxDMA(); err != nil {
				println("RX DMA error:", err.Error())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Initialize network stack

	// Main network processing loop
	println("\nStarting network stack...")
	for {
		time.Sleep(1 * time.Millisecond)
	}
}

// initRMII initializes the RMII interface with PIO and DMA
// Reference: netif_rmii_ethernet_low_init() from rmii_ethernet.c
func initRMII(Pio *pio.PIO) (*piolib.RMII, error) {
	smTx, err := Pio.ClaimStateMachine()
	if err != nil {
		return nil, err
	}
	smRx, err := Pio.ClaimStateMachine()
	if err != nil {
		return nil, err
	}
	// Configure RMII
	cfg := piolib.RMIIConfig{
		TxRx: piolib.RMIITxRxConfig{
			TxPin:     pinTxBase,
			RxPin:     pinRxBase,
			CRSDVPin:  pinCRSDV,
			RefClkPin: pinRefClk,
		},
		MDIO:         pinMDIO,
		MDC:          pinMDC,
		RxBufferSize: 2048,
		TxBufferSize: 2048,
	}
	rmii, err := piolib.NewRMII(smTx, smRx, cfg)
	if err != nil {
		return nil, err
	}
	return rmii, nil
}

// Utility functions for formatting

func formatHex16(val uint16) string {
	fwd := []byte{'0', 'x'}
	fwd = appendHex(fwd, byte(val>>8))
	fwd = appendHex(fwd, byte(val))
	return unsafe.String(&fwd[0], len(fwd))
}

// waitForLink waits for the PHY link to come up
func waitForLink(rmii *piolib.RMII) {
	println("Reading PHY registers for diagnostics...")

	// Read PHY ID registers
	id1, err1 := rmii.MDIORead(rmii.PHYAddr(), 2)
	id2, err2 := rmii.MDIORead(rmii.PHYAddr(), 3)
	if err1 == nil && err2 == nil {
		println("  PHY ID1:", formatHex16(id1))
		println("  PHY ID2:", formatHex16(id2))
	}

	// Read control register
	ctrl, errCtrl := rmii.MDIORead(rmii.PHYAddr(), 0)
	if errCtrl == nil {
		println("  Control Reg (0):", formatHex16(ctrl))
	}

	attempt := 0
	for {
		// Read PHY Basic Status Register (register 1)
		status, err := rmii.MDIORead(rmii.PHYAddr(), 1)
		if err != nil {
			println("Error reading PHY status:", err.Error())
			time.Sleep(100 * time.Millisecond)
			continue
		}

		attempt++
		if attempt%10 == 0 {
			println("  Status Reg (1):", formatHex16(status), "- Link bit:", (status>>2)&1)

			// Read more diagnostic info
			if attempt%30 == 0 {
				// Read auto-neg advertisement (reg 4)
				anar, _ := rmii.MDIORead(rmii.PHYAddr(), 4)
				println("  Auto-Neg Adv (4):", formatHex16(anar))

				// Read auto-neg link partner (reg 5)
				anlpar, _ := rmii.MDIORead(rmii.PHYAddr(), 5)
				println("  Link Partner (5):", formatHex16(anlpar))
			}
		}

		// Check link status bit (bit 2)
		if status&0x04 != 0 {
			println("  Link established! Final status:", formatHex16(status))
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func appendHexSep(dst, mac []byte, sep byte) []byte {
	for i := range mac {
		dst = appendHex(dst, mac[i])
		if sep != 0 && i != len(mac)-1 {
			dst = append(dst, sep)
		}
	}
	return dst
}

func appendHex(dst []byte, b byte) []byte {
	const hexChars = "0123456789abcdef"
	return append(dst, hexChars[b>>4], hexChars[b&0xf])
}

func appendDec(dst []byte, b byte) []byte {
	return strconv.AppendInt(dst, int64(b), 10)
}

func appendDecSep(dst []byte, data []byte, sep byte) []byte {
	for i := range data {
		dst = appendDec(dst, data[i])
		if sep != 0 && i != len(data)-1 {
			dst = append(dst, sep)
		}
	}
	return dst
}
