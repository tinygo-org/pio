package main

import (
	"machine"
	"time"
)

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

func main() {
	time.Sleep(2 * time.Second)
	println("start program")
	const zmdio = true
	const mdioMaxBaud = 50_000
	const mdioMinSleep = time.Second / mdioMaxBaud
	var mdio MDIO
	var addrsraw [32]uint8
	for baud := 10_000; baud <= mdioMaxBaud; baud += 1000 {
		mdio.Configure(pinMDIO, pinMDC, baud, zmdio)
		found := mdio.FindPHYs(addrsraw[:])
		if found <= 0 {
			println("no addrs found mdio baud", baud)
			continue
		}
		println("found ", found, "PHYs @ baud", baud)
		for i := range found {
			println("\tPHY @", addrsraw[i])
		}
		time.Sleep(200 * time.Millisecond)
	}
}
