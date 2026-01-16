package main

import (
	"errors"
	"time"
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
	BMCRAddr = 0x00

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
	BMSRAddr = 0x01

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

// ANAR represents the Auto-Negotiation Advertisement Register at address 0x04.
// ANLPAR (Link Partner Ability Register at 0x05) shares the same bit layout.
// Reference: IEEE 802.3 Clause 28.2.4.1
type ANAR uint16

const (
	ANARAddr   = 0x04
	ANLPARAddr = 0x05
	ANERAddr   = 0x06

	ANARSelector    ANAR = 0x001f // Protocol selector (802.3 = 0x01)
	ANAR10Half      ANAR = 0x0020 // 10BASE-T half-duplex
	ANAR10Full      ANAR = 0x0040 // 10BASE-T full-duplex
	ANAR100Half     ANAR = 0x0080 // 100BASE-TX half-duplex
	ANAR100Full     ANAR = 0x0100 // 100BASE-TX full-duplex
	ANAR100BaseT4   ANAR = 0x0200 // 100BASE-T4
	ANARPause       ANAR = 0x0400 // Pause capability
	ANARPauseAsym   ANAR = 0x0800 // Asymmetric pause
	ANARRemoteFault ANAR = 0x2000 // Remote fault
	ANARAck         ANAR = 0x4000 // Acknowledge (ANLPAR only)
	ANARNextPage    ANAR = 0x8000 // Next page capable
)

// LinkMode represents the negotiated Ethernet link speed and duplex mode.
//
// Naming convention:
//   - H/HDX: Half-duplex (one direction at a time)
//   - F/FDX: Full-duplex (simultaneous bidirectional)
//   - T4: 100BASE-T4 (100Mbps over 4 twisted pairs, legacy)
//   - G: Gigabit, implies number is multiplied by 1000 (1G=1000M)
//
//go:generate stringer -type=LinkMode -linecomment
type LinkMode uint8

const (
	LinkDown    LinkMode = iota // down
	Link10HDX                   // 10M-H
	Link10FDX                   // 10M-F
	Link100HDX                  // 100M-H
	Link100FDX                  // 100M-F
	Link100T4                   // 100M-T4
	Link1000HDX                 // 1000M-H
	Link1000FDX                 // 1000M-F

	// Clause 45 speeds (10Gbps+, full-duplex only):

	Link2500FDX // 2.5G-F
	Link5GFDX   // 5G-F
	Link10GFDX  // 10G-F
	Link25GFDX  // 25G-F
	Link40GFDX  // 40G-F
	Link100GFDX // 100G-F
)

// SpeedMbps returns the link speed in megabits per second.
func (lm LinkMode) SpeedMbps() int {
	switch lm {
	case Link10HDX, Link10FDX:
		return 10
	case Link100HDX, Link100FDX, Link100T4:
		return 100
	case Link1000HDX, Link1000FDX:
		return 1000
	case Link2500FDX:
		return 2500
	case Link5GFDX:
		return 5000
	case Link10GFDX:
		return 10_000
	case Link25GFDX:
		return 25_000
	case Link40GFDX:
		return 40_000
	case Link100GFDX:
		return 100_000
	default:
		return 0
	}
}

// IsFullDuplex returns true if the link mode is full duplex.
func (lm LinkMode) IsFullDuplex() bool {
	switch lm {
	case Link10FDX, Link100FDX, Link1000FDX,
		Link2500FDX, Link5GFDX, Link10GFDX, Link25GFDX, Link40GFDX, Link100GFDX:
		return true
	default:
		return false
	}
}

type PHY struct {
	mdio       MDIOBus
	phyaddr    uint8
	isClause45 uint8
}

func (rmii *PHY) IsClause45() bool {
	return rmii.isClause45 == 1
}

func (rmii *PHY) PHYAddr() uint8 {
	return rmii.phyaddr
}

func (rmii *PHY) BasicControl() (BMCR, error) {
	ctl, err := rmii.rread(BMCRAddr)
	return BMCR(ctl), err
}

func (rmii *PHY) BasicStatus() (BMSR, error) {
	stat, err := rmii.rread(BMSRAddr)
	return BMSR(stat), err
}

func (rmii *PHY) SetControlEnable(b bool) error {
	ctl, err := rmii.BasicControl()
	if err != nil {
		return err
	}
	ctl |= BMCRANEnable
	err = rmii.rwrite(regBasicControl, uint16(ctl))
	if err != nil {
		return err
	}
	ctl, err = rmii.BasicControl()
	if (ctl&BMCRANEnable != 0) != b {
		return errors.New("unable to set control enable bit")
	}
	return nil
}

func (rmii *PHY) ID1() (uint16, error) {
	return rmii.rread(regPhyId1)
}

func (rmii *PHY) ID2() (uint16, error) {
	return rmii.rread(regPhyId2)
}

// ResetPHY performs a software reset and waits for completion.
func (rmii *PHY) ResetPHY() (err error) {
	err = rmii.rwrite(BMCRAddr, uint16(BMCRReset))
	if err != nil {
		return err
	}
	// Wait for reset to complete (bit self-clears).
	// IEEE 802.3 allows up to 500ms.
	const maxPolls = 50
	const resetTimeout = 500 * time.Millisecond // As per standard.
	var ctl BMCR
	for i := 0; i < maxPolls; i++ {
		time.Sleep(resetTimeout / maxPolls)
		ctl, err = rmii.BasicControl()
		if err != nil {
			continue
		}
		if ctl&BMCRReset == 0 {
			return nil
		}
	}
	if err != nil {
		return err
	}
	return errors.New("PHY reset timeout")
}

// EnableAutoNeg enables auto-negotiation and restarts it.
func (rmii *PHY) EnableAutoNeg() error {
	ctl, err := rmii.BasicControl()
	if err != nil {
		return err
	}
	ctl |= BMCRANEnable | BMCRANRestart
	return rmii.rwrite(BMCRAddr, uint16(ctl))
}

// IsLinkUp returns true if link is established.
func (rmii *PHY) IsLinkUp() (bool, error) {
	status, err := rmii.BasicStatus()
	if err != nil {
		return false, err
	}
	return status&BMSRLinkStatus != 0, nil
}

func (rmii *PHY) rread(regaddr uint16) (uint16, error) {
	return rmii.mdio.Read(rmii.phyaddr, rmii.isClause45, regaddr)
}
func (rmii *PHY) rwrite(regaddr, value uint16) error {
	return rmii.mdio.Write(rmii.phyaddr, rmii.isClause45, regaddr, value)
}

// NegotiatedLink returns the auto-negotiated link mode using standard MII registers.
// Returns LinkMode based on ANAR (our advertisement) AND ANLPAR (link partner ability).
// Priority order per IEEE 802.3 Annex 28B.3.
func (phy *PHY) NegotiatedLink() (LinkMode, error) {
	// First check if auto-negotiation is complete
	status, err := phy.rread(BMSRAddr)
	if err != nil {
		return LinkDown, err
	}
	if BMSR(status)&BMSRANComplete == 0 {
		return LinkDown, errors.New("auto-negotiation not complete")
	}

	// Read our advertisement
	anar, err := phy.rread(ANARAddr)
	if err != nil {
		return LinkDown, err
	}

	// Read link partner's advertisement
	anlpar, err := phy.rread(ANLPARAddr)
	if err != nil {
		return LinkDown, err
	}

	// Common capabilities = what both sides support
	common := ANAR(anar) & ANAR(anlpar)

	// Select highest common capability (priority order per IEEE 802.3 Annex 28B.3)
	switch {
	case common&ANAR100Full != 0:
		return Link100FDX, nil
	case common&ANAR100BaseT4 != 0:
		return Link100T4, nil
	case common&ANAR100Half != 0:
		return Link100HDX, nil
	case common&ANAR10Full != 0:
		return Link10FDX, nil
	case common&ANAR10Half != 0:
		return Link10HDX, nil
	default:
		return LinkDown, nil
	}
}
