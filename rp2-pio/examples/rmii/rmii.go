package main

import (
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

type RMII struct {
	rxtx piolib.RMIITxRx
	PHY
}

type RMIIConfig struct {
	PIO       *pio.PIO
	TxPinBase machine.Pin
	RxPinBase machine.Pin
	CRSDV     machine.Pin
	RefClk    machine.Pin

	MDIOPin machine.Pin
	MDCPin  machine.Pin
	Baud    int
}

func (rmii *RMII) Configure(cfg RMIIConfig) error {
	var mdio MDIO // We use default bitbang for example. See github.com/soypat/lneto/phy for better patterns.
	mdio.Configure(cfg.MDIOPin, cfg.MDCPin, 50_000, true)
	rmii.PHY.mdio = &mdio
	rmii.PHY.isClause45 = 0
	err := rmii.SetFirstAddr()
	if err != nil {
		return err
	}
	err = rmii.ResetPHY()
	if err != nil {
		return err
	}
	txSM, err := cfg.PIO.ClaimStateMachine()
	if err != nil {
		return err
	}
	rxSM, err := cfg.PIO.ClaimStateMachine()
	if err != nil {
		txSM.Unclaim()
		return err
	}
	rxtx, err := piolib.NewRMIITxRx(txSM, rxSM, piolib.RMIITxRxConfig{
		Baud:      uint32(cfg.Baud),
		TxPin:     cfg.TxPinBase,
		RxPin:     cfg.RxPinBase,
		CRSDVPin:  cfg.CRSDV,
		RefClkPin: cfg.RefClk,
	})
	if err != nil {
		txSM.Unclaim()
		rxSM.Unclaim()
		return err
	}
	rmii.rxtx = *rxtx
	return nil
}

func (rmii *RMII) SetFirstAddr() error {
	var addrs [32]uint8
	_, err := FindClause22PHYs(rmii.mdio, addrs[:])
	if err != nil {
		return err
	}
	rmii.phyaddr = addrs[0]
	return nil
}

// LinkSpeed returns the negotiated link speed string (LAN8720-specific).
func (rmii *RMII) LinkSpeed() (string, error) {
	val, err := rmii.rread(regPhySpecialScontrolStatus)
	if err != nil {
		return "", err
	}
	// Bits [4:2] = Speed indication
	speed := (val >> 2) & 0x07
	switch speed {
	case 0x01:
		return "10Mbps half-duplex", nil
	case 0x05:
		return "10Mbps full-duplex", nil
	case 0x02:
		return "100Mbps half-duplex", nil
	case 0x06:
		return "100Mbps full-duplex", nil
	default:
		return "unknown", nil
	}
}
