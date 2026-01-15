package main

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
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
	var rmii RMII
	err := rmii.Configure(RMIIConfig{
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
}
