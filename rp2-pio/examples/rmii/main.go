package main

import (
	"errors"
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
	cfg := piolib.RMIIConfig{
		TxRx: piolib.RMIITxRxConfig{
			TxPin:     pinTxBase,
			RxPin:     pinRxBase,
			CRSDVPin:  pinCRSDV,
			RefClkPin: pinRefClk,
		},
		NoZMDIO:      false,
		MDIO:         pinMDIO,
		MDC:          pinMDC,
		RxBufferSize: 2048,
		TxBufferSize: 2048,
	}
	// Sleep to allow serial monitor to connect
	time.Sleep(2 * time.Second)
	println("=== LAN 8720 RMII ===")
	device, err := NewLAN8270(pio.PIO0, cfg)
	if err != nil {
		panic(err)
	}
	// Init Loop:
	for {
		err = device.Init()
		if err == nil {
			break
		}
		println("init failed:", err.Error())
		println("retrying soon...")
		time.Sleep(6 * time.Second)
	}
	status, err := device.Status()
	if err != nil {
		panic("status: " + err.Error())
	}
	ctl, _ := device.BasicControl()
	println("status", formatHex16(uint16(status)), "islinked", status.IsLinked())
	println("regctl", formatHex16(uint16(ctl)), "isenabled", ctl.IsEnabled())
	println("PHY ID1:", device.id1, "ID2:", device.id2)

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
		NoZMDIO:      false,
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

const (
	regBasicControl = 0x00
	regBasicStatus  = 0x01
	regPhyId1       = 0x02
	regPhyId2       = 0x03

	regAutoNegotiationAdvertisement      = 0x04
	regAutoNegotiationLinkPartnerAbility = 0x05
	regAutoNegotiationExpansion          = 0x05
	regModeControlStatus                 = 0x11
	regSpecialModes                      = 0x12
	regSymbolErorCounter                 = 0x1a
	regSpecialControlStatusIndications   = 0x1b
	regIRQSourceFlag                     = 0x1d
	regIRQMask                           = 0x1e
	regPhySpecialScontrolStatus          = 0x1f
)

type LAN8720 struct {
	bus      *piolib.RMII
	smiaddr  uint8
	id1, id2 uint16
}

func NewLAN8270(Pio *pio.PIO, cfg piolib.RMIIConfig) (*LAN8720, error) {
	smTx, err := Pio.ClaimStateMachine()
	if err != nil {
		return nil, err
	}
	smRx, err := Pio.ClaimStateMachine()
	if err != nil {
		return nil, err
	}
	// Configure RMII

	rmii, err := piolib.NewRMII(smTx, smRx, cfg)
	if err != nil {
		return nil, err
	}
	return &LAN8720{bus: rmii}, nil
}

type status uint16
type control uint16

func (c *control) SetEnabled(b bool) {
	*c &^= 1 << 15
	if b {
		*c |= 1 << 15
	}
}
func (c control) IsEnabled() bool {
	return c&(1<<15) != 0
}

func (s status) IsLinked() bool {
	return s&(1<<2) != 0
}

func (lan *LAN8720) Status() (status, error) {
	stat, err := lan.readReg(regBasicStatus)
	return status(stat), err
}

func (lan *LAN8720) BasicControl() (control, error) {
	ct, err := lan.readReg(regBasicControl)
	return control(ct), err
}

func (lan *LAN8720) Init() error {
	const maxAddr = 31
	lan.smiaddr = 255
	for addr := uint8(0); addr <= maxAddr; addr++ {
		val, err := lan.bus.MDIORead(addr, 0)
		if err != nil {
			continue
		}
		if val != 0xffff && val != 0x0000 {
			lan.smiaddr = addr
			break
		}
		time.Sleep(150 * time.Microsecond)
	}
	if lan.smiaddr > maxAddr {
		return errors.New("no PHY found via addr scanning")
	}
	ctl, err := lan.BasicControl()
	if err != nil {
		return errors.New("failed reading basic control: " + err.Error())
	}
	ctl.SetEnabled(true)
	err = lan.writeReg(regBasicControl, uint16(ctl))
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	ctl, err = lan.BasicControl()

	if err != nil {
		return err
	} else if ctl.IsEnabled() {
		println("want ctl bit 16, got:", formatHex16(uint16(ctl)))
		return errors.New("lan8720 reset failed")
	}
	lan.id1, err = lan.readReg(regPhyId1)
	if err != nil {
		return err
	}
	lan.id2, err = lan.readReg(regPhyId2)
	if err != nil {
		return err
	}
	return nil
}

func (lan *LAN8720) readReg(reg uint8) (uint16, error) {
	return lan.bus.MDIORead(lan.smiaddr, reg)
}

func (lan *LAN8720) writeReg(reg uint8, value uint16) error {
	return lan.bus.MDIOWrite(lan.smiaddr, reg, value)
}

// Utility functions for formatting

func formatHex16(val uint16) string {
	fwd := []byte{'0', 'x'}
	fwd = appendHex(fwd, byte(val>>8))
	fwd = appendHex(fwd, byte(val))
	return unsafe.String(&fwd[0], len(fwd))
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
