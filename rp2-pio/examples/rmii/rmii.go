package main

import (
	"errors"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

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

// BMCR represents the Basic Mode Control Register at address 0x00.
// Reference: IEEE 802.3 Clause 22.2.4.1
type BMCR uint16

const (
	BMCRSpeed1000  BMCR = 0x0040 // MSB of Speed (1000Mbps)
	BMCRCollision  BMCR = 0x0080 // Collision test
	BMCRFullDuplex BMCR = 0x0100 // Full duplex mode
	BMCRANRestart  BMCR = 0x0200 // Restart auto-negotiation
	BMCRIsolate    BMCR = 0x0400 // Isolate PHY from MII
	BMCRPowerDown  BMCR = 0x0800 // Power down PHY
	BMCRANEnable   BMCR = 0x1000 // Enable auto-negotiation
	BMCRSpeed100   BMCR = 0x2000 // Select 100Mbps
	BMCRLoopback   BMCR = 0x4000 // Enable TXD loopback
	BMCRReset      BMCR = 0x8000 // Software reset (self-clearing)
)

// BMSR represents the Basic Mode Status Register at address 0x01.
// Reference: IEEE 802.3 Clause 22.2.4.2
type BMSR uint16

const (
	BMSRExtCap      BMSR = 0x0001 // Extended register capability
	BMSRJabber      BMSR = 0x0002 // Jabber detected
	BMSRLinkStatus  BMSR = 0x0004 // Link status (1=up)
	BMSRANCap       BMSR = 0x0008 // Auto-negotiation capable
	BMSRRemoteFault BMSR = 0x0010 // Remote fault detected
	BMSRANComplete  BMSR = 0x0020 // Auto-negotiation complete
	BMSRNoPreamble  BMSR = 0x0040 // Preamble suppression capable
	BMSRExtStatus   BMSR = 0x0100 // Extended status in register 15
	BMSR100Half2    BMSR = 0x0200 // 100BASE-T2 half-duplex capable
	BMSR100Full2    BMSR = 0x0400 // 100BASE-T2 full-duplex capable
	BMSR10Half      BMSR = 0x0800 // 10Mbps half-duplex capable
	BMSR10Full      BMSR = 0x1000 // 10Mbps full-duplex capable
	BMSR100Half     BMSR = 0x2000 // 100Mbps half-duplex capable
	BMSR100Full     BMSR = 0x4000 // 100Mbps full-duplex capable
	BMSR100Base4    BMSR = 0x8000 // 100BASE-T4 capable
)

type RMII struct {
	rxtx    piolib.RMIITxRx
	mdio    MDIO
	phyaddr uint8
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
	rmii.mdio.Configure(cfg.MDIOPin, cfg.MDCPin, 50_000, true)
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

func (rmii *RMII) PHYAddr() uint8 {
	return rmii.phyaddr
}

func (rmii *RMII) SetFirstAddr() error {
	var addrs [32]uint8
	nFound := rmii.mdio.FindPHYs(addrs[:])
	if nFound <= 0 {
		return errors.New("did not find any PHY on MDIO line")
	}
	rmii.phyaddr = addrs[0]
	return nil
}

func (rmii *RMII) BasicControl() (BMCR, error) {
	ctl, err := rmii.mdio.Read(rmii.phyaddr, regBasicControl)
	return BMCR(ctl), err
}

func (rmii *RMII) BasicStatus() (BMSR, error) {
	stat, err := rmii.mdio.Read(rmii.phyaddr, regBasicStatus)
	return BMSR(stat), err
}

func (rmii *RMII) ResetBasicControl() error {
	return rmii.mdio.Write(rmii.phyaddr, regBasicControl, uint16(BMCRReset))
}

func (rmii *RMII) SetControlEnable(b bool) error {
	ctl, err := rmii.BasicControl()
	if err != nil {
		return err
	}
	ctl |= BMCRANEnable
	err = rmii.mdio.Write(rmii.phyaddr, regBasicControl, uint16(ctl))
	if err != nil {
		return err
	}
	ctl, err = rmii.BasicControl()
	if (ctl&BMCRANEnable != 0) != b {
		return errors.New("unable to set control enable bit")
	}
	return nil
}

func (rmii *RMII) ID1() (uint16, error) {
	return rmii.mdio.read(rmii.phyaddr, regPhyId1)
}

func (rmii *RMII) ID2() (uint16, error) {
	return rmii.mdio.read(rmii.phyaddr, regPhyId2)
}

// ResetPHY performs a software reset and waits for completion.
func (rmii *RMII) ResetPHY() error {
	err := rmii.mdio.Write(rmii.phyaddr, regBasicControl, uint16(BMCRReset))
	if err != nil {
		return err
	}
	// Wait for reset to complete (bit self-clears).
	// IEEE 802.3 allows up to 500ms.
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		ctl, err := rmii.BasicControl()
		if err != nil {
			continue
		}
		if ctl&BMCRReset == 0 {
			return nil
		}
	}
	return errors.New("PHY reset timeout")
}

// EnableAutoNeg enables auto-negotiation and restarts it.
func (rmii *RMII) EnableAutoNeg() error {
	ctl, err := rmii.BasicControl()
	if err != nil {
		return err
	}
	ctl |= BMCRANEnable | BMCRANRestart
	return rmii.mdio.Write(rmii.phyaddr, regBasicControl, uint16(ctl))
}

// IsLinkUp returns true if link is established.
func (rmii *RMII) IsLinkUp() (bool, error) {
	status, err := rmii.BasicStatus()
	if err != nil {
		return false, err
	}
	return status&BMSRLinkStatus != 0, nil
}

// LinkSpeed returns the negotiated link speed string (LAN8720-specific).
func (rmii *RMII) LinkSpeed() (string, error) {
	val, err := rmii.mdio.Read(rmii.phyaddr, regPhySpecialScontrolStatus)
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
