//go:build rp2040 || rp2350

package main

import (
	"errors"
	"machine"
	"time"
)

const (
	mdioRead  = 0b10
	mdioWrite = 0b01
	miaddrc45 = 1 << 30
	c45bit    = 1 << 15
	c45Addr   = c45bit | 0b00
	c45Read   = c45bit | 0b11
	c45Write  = c45bit | 0b01
)

// MDIOBus is a HAL for MDIO bus access supporting both Clause 22 and Clause 45 devices.
// Implementations should use devaddr to select the framing:
//   - devaddr=0: Clause 22 framing (devaddr ignored in transaction)
//   - devaddr>=1: Clause 45 framing (PMA/PMD=1, WIS=2, PCS=3, PHY XS=4, DTE XS=5, AN=7)
//
// Register address range: Clause 22 uses 0-31, Clause 45 uses 0-65535.
// Invalid combinations of devaddr and regAddr may or may not return an error
// depending on the implementation or result in undefined behavior.
// To avoid this wrap your MDIOBus interfaces with a wrapper type that checks validity of ranges.
type MDIOBus interface {
	// Read reads a 16-bit register from the PHY.
	Read(phyAddr, devAddr uint8, regAddr uint16) (value uint16, err error)
	// Write writes a 16-bit value to a PHY register.
	Write(phyAddr, devAddr uint8, regAddr, value uint16) error
}

// FindPHYs finds all regular non-clause45 PHYs on the MDIO bus and writes them to dst.
// FindClause22PHYs returns error only if unable to find no PHYs.
func FindClause22PHYs(mdio MDIOBus, dst []uint8) (n int, err error) {
	const maxAddr = 31
	const regBasicStatus = 0x01
	if len(dst) < 32 {
		return -1, errors.New("require buffer length 32 for FindPHYs")
	}
	n = 0
	for addr := uint8(0); addr <= maxAddr; addr++ {
		// Future proofing for supported clause 45.
		// Check PMA/PMD device (DEVAD 1), register 0 (control)
		val, err := mdio.Read(addr, 0, BMSRAddr)
		if err != nil {
			continue
		}
		// Basic status has some bits that must be zero and one, so if this check fails then we know its a bad address.
		if val != 0xffff && val != 0x0000 {
			dst[n] = addr
			n++
		}
		time.Sleep(150 * time.Microsecond)
	}
	if n <= 0 {
		err = errors.New("no phy found")
	}
	return n, err
}

var _ MDIOBus = (*MDIOBitBang)(nil) // compile time guarantee of interface implementation.

// MDIOBitBang provides a software defined(bitbang) MDIO/MDC management interface for PHY register access
// as the STA (Management station, this implementation) which communicates to the PHY (Physical layer device).
// Inspired by linux/v3.13.1/source/drivers/net/phy/mdio-bitbang.c
type MDIOBitBang struct {
	zmdio  bool
	data   machine.Pin
	clk    machine.Pin
	_delay time.Duration
}

func (m *MDIOBitBang) Configure(dataPin, clkPin machine.Pin, baud int, zmdio bool) {
	m._delay = time.Second / time.Duration(baud) / 2
	m._delay = max(m._delay, time.Nanosecond*340) // 300ns is max turnaround time according to MDIO spec. We give some leeway.
	m.zmdio = zmdio
	m.data = dataPin
	m.clk = clkPin
	// m.reset()
}

// Read performs regular read of a PHY's register.
func (m *MDIOBitBang) Read(phyAddr, devAddr uint8, regAddr uint16) (uint16, error) {
	isC45 := devAddr != 0
	if isC45 {
		m.cmdAddr2(phyAddr, devAddr, regAddr)
		m.cmd(c45Read, phyAddr, devAddr)
	} else {
		m.cmd(mdioRead, phyAddr, uint8(regAddr))
	}
	m.setDir(false)
	// Check turnaround bit, PHY should drive it to zero.
	if m.getBit() {
		// PHY did not drive low, as would be expected.
		// Ensure flush:
		for range 32 {
			m.getBit()
		}
		return 0xffff, errors.New("PHY did not drive turnaround low")
	}
	ret := m.getNum(16)
	m.getBit()
	return ret, nil
}

// Read performs regular read of a PHY's register.
func (m *MDIOBitBang) Write(phyAddr, devAddr uint8, regAddr, value uint16) error {
	isC45 := devAddr != 0
	if isC45 {
		m.cmdAddr2(phyAddr, devAddr, regAddr)
		m.cmd(c45Write, phyAddr, devAddr)
	} else {
		m.cmd(mdioWrite, phyAddr, uint8(regAddr))
	}
	// send turnaround (10)
	m.sendBit(true)
	m.sendBit(false)

	m.sendNum(value, 16)
	m.setDir(false)
	m.getBit()
	return nil
}

func (m *MDIOBitBang) cmdAddr2(phy, dev uint8, reg uint16) {
	m.cmd(c45Addr, phy, dev)
	// turnaround 10.
	m.sendBit(true)
	m.sendBit(false)

	m.sendNum(reg, 16)
	m.setDir(false)
	m.getBit()
}

func (m *MDIOBitBang) cmd(op uint16, phy uint8, reg uint8) {
	const writeDir = true
	m.setDir(writeDir)
	// Preamble, 32 bits of 1.
	for range 32 {
		m.sendBit(true)
	}
	// Start of frame: 01
	// Clause 45 op uses 00=start, 11=read, 10=write
	m.sendBit(false)
	m.sendBit(op&c45bit == 0)

	m.sendBit((op>>1)&1 != 0)
	m.sendBit((op>>0)&1 != 0)
	m.sendNum(uint16(phy), 5)
	m.sendNum(uint16(reg), 5)
}

func (m *MDIOBitBang) sendNum(val uint16, bits int) {
	for i := bits - 1; i >= 0; i-- {
		m.sendBit((val>>i)&1 != 0)
	}
}

func (m *MDIOBitBang) getNum(bits int) (ret uint16) {
	for i := bits - 1; i >= 0; i-- {
		ret <<= 1
		ret |= uint16(b2u8(m.getBit()))
	}
	return ret
}

// MDIO low-level clock operations
// Reference: https://github.com/sandeepmistry/pico-rmii-ethernet/blob/main/examples/httpd/main.c
// Reference: netif_rmii_ethernet_mdio_clock_out() and netif_rmii_ethernet_mdio_clock_in()
// from rmii_ethernet.c

// reset sets the resting voltages on the MDIO bus.
func (m *MDIOBitBang) reset() {
	// Set values BEFORE configuring as output (avoids glitches)
	m.clk.Low()
	m.clk.Configure(machine.PinConfig{Mode: machine.PinOutput})
	if m.zmdio {
		// Open-drain: release to pullup
		m.data.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	} else {
		// Push-pull: drive high
		m.data.High()
		m.data.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}
}

// setDir configures pins preparing for write/read operations.
func (m *MDIOBitBang) setDir(outWrite bool) {
	if outWrite {
		if m.zmdio {
			// In zmdio mode, mdioSet() handles direction
			// Just ensure we're not actively driving
			m.data.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
		} else {
			// Configure for driving MDIO
			m.data.Configure(machine.PinConfig{Mode: machine.PinOutput})
		}
	} else {
		// Release MDIO for PHY to drive
		m.data.Configure(machine.PinConfig{Mode: machine.PinInput})
	}
	m.clk.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

// mdioClockOut outputs a bit on MDIO while pulsing MDC clock.
func (m *MDIOBitBang) mdioClockOut(bit bool) {
	m.clk.Low()
	m.delay()
	m.mdioSet(bit)
	m.delay()
	m.clk.High()
	m.delay()
}

// delay sleeps for half a clock cycle.
func (m *MDIOBitBang) delay() {
	time.Sleep(m._delay)
}

func (m *MDIOBitBang) mdioSet(b bool) {
	if m.zmdio {
		if b {
			m.mdioZHigh()
		} else {
			m.mdioLow()
		}
	} else {
		m.data.Set(b)
	}
}

func (m *MDIOBitBang) mdioZHigh() {
	// RMII z pin level means high impedance, pull up resistor.
	m.data.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
}

func (m *MDIOBitBang) mdioLow() {
	// RMII 0 pin level sets as output
	m.data.Low()
	m.data.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func (m *MDIOBitBang) sendBit(b bool) {
	m.mdioSet(b)
	m.delay()
	m.clk.High()
	m.delay()
	m.clk.Low()
}

func (m *MDIOBitBang) getBit() bool {
	m.delay()
	m.clk.High()
	m.delay()
	m.clk.Low()
	return m.data.Get()
}

func b2u8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
