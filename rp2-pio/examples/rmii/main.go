package main

import (
	"machine"
	"time"

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
	println("  MAC:", formatMAC(macAddr[:]))
	println("  IP:", formatIP(ipAddr[:]))
	println("  Netmask:", formatIP(netmask[:]))
	println("  Gateway:", formatIP(gateway[:]))
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

// waitForLink waits for the PHY link to come up
func waitForLink(rmii *piolib.RMII) {
	for {
		// Read PHY Basic Status Register (register 1)
		status, err := rmii.MDIORead(rmii.PHYAddr(), 1)
		if err != nil {
			println("Error reading PHY status:", err.Error())
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Check link status bit (bit 2)
		if status&0x04 != 0 {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Utility functions for formatting

func formatMAC(mac []byte) string {
	if len(mac) != 6 {
		return "invalid"
	}
	return formatHex(mac[0]) + ":" + formatHex(mac[1]) + ":" + formatHex(mac[2]) + ":" +
		formatHex(mac[3]) + ":" + formatHex(mac[4]) + ":" + formatHex(mac[5])
}

func formatIP(ip []byte) string {
	if len(ip) != 4 {
		return "invalid"
	}
	return formatDec(ip[0]) + "." + formatDec(ip[1]) + "." + formatDec(ip[2]) + "." + formatDec(ip[3])
}

func formatHex(b byte) string {
	const hexChars = "0123456789abcdef"
	return string([]byte{hexChars[b>>4], hexChars[b&0x0f]})
}

func formatDec(b byte) string {
	if b == 0 {
		return "0"
	}

	var buf [3]byte
	i := 2
	for b > 0 && i >= 0 {
		buf[i] = '0' + (b % 10)
		b /= 10
		i--
	}
	return string(buf[i+1:])
}
