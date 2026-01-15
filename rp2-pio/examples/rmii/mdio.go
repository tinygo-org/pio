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

// MDIO provides MDIO/MDC management interface for PHY register access
// as the STA (Management station, this implementation) which communicates to the PHY (Physical layer device).
// Inspired by linux/v3.13.1/source/drivers/net/phy/mdio-bitbang.c
type MDIO struct {
	zmdio  bool
	data   machine.Pin
	clk    machine.Pin
	_delay time.Duration
}

func (m *MDIO) Configure(dataPin, clkPin machine.Pin, baud int, zmdio bool) {
	m._delay = time.Second / time.Duration(baud) / 2
	m._delay = max(m._delay, time.Nanosecond*340) // 300ns is max turnaround time according to MDIO spec. We give some leeway.
	m.zmdio = zmdio
	m.data = dataPin
	m.clk = clkPin
	m.reset()
}

// FindPHYs finds all regular non-clause45 PHYs on the MDIO bus and writes them to dst.
func (m *MDIO) FindPHYs(dst []uint8) int {
	const maxAddr = 31
	const regBasicStatus = 0x01
	if len(dst) < 32 {
		panic("require buffer length 32 for FindAddrs")
	}
	written := 0
	for addr := uint8(0); addr <= maxAddr; addr++ {
		val, err := m.read(addr, regBasicStatus)
		if err != nil {
			continue
		}
		// Basic status has some bits that must be zero and one, so if this check fails then we know its a bad address.
		if val != 0xffff && val != 0x0000 {
			dst[written] = addr
			written++
		}
		time.Sleep(150 * time.Microsecond)
	}
	return written
}

// Read performs regular read of a PHY's register.
func (m *MDIO) Read(phy, reg uint8) (uint16, error) {
	return m.read(phy, uint32(reg))
}

// Read performs regular read of a PHY's register.
func (m *MDIO) Write(phy, reg uint8, value uint16) error {
	m.write(phy, uint32(reg), value)
	return nil
}

func (m *MDIO) read(phy uint8, reg uint32) (uint16, error) {
	if reg&miaddrc45 != 0 {
		reg = m.cmdAddr(phy, reg)
		m.cmd(c45Read, phy, uint8(reg))
	} else {
		m.cmd(mdioRead, phy, uint8(reg))
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

func (m *MDIO) write(phy uint8, reg uint32, value uint16) {
	if reg&miaddrc45 != 0 {
		reg = m.cmdAddr(phy, reg)
		m.cmd(c45Write, phy, uint8(reg))
	} else {
		m.cmd(mdioWrite, phy, uint8(reg))
	}
	// send turnaround (10)
	m.sendBit(true)
	m.sendBit(false)

	m.sendNum(value, 16)
	m.setDir(false)
	m.getBit()
}

func (m *MDIO) cmdAddr(phy uint8, addr uint32) uint32 {
	devAddr := (addr >> 16) & 0x1f
	reg := addr & 0xffff
	m.cmd(c45Addr, phy, uint8(devAddr))
	// turnaround 10.
	m.sendBit(true)
	m.sendBit(false)

	m.sendNum(uint16(reg), 16)
	m.setDir(false)
	m.getBit()
	return devAddr
}

func (m *MDIO) cmd(op uint16, phy uint8, reg uint8) {
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

func (m *MDIO) sendNum(val uint16, bits int) {
	for i := bits - 1; i >= 0; i-- {
		m.sendBit((val>>i)&1 != 0)
	}
}

func (m *MDIO) getNum(bits int) (ret uint16) {
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
func (m *MDIO) reset() {
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
func (m *MDIO) setDir(outWrite bool) {
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
func (m *MDIO) mdioClockOut(bit bool) {
	m.clk.Low()
	m.delay()
	m.mdioSet(bit)
	m.delay()
	m.clk.High()
	m.delay()
}

// delay sleeps for half a clock cycle.
func (m *MDIO) delay() {
	time.Sleep(m._delay)
}

func (m *MDIO) mdioSet(b bool) {
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

func (m *MDIO) mdioZHigh() {
	// RMII z pin level means high impedance, pull up resistor.
	m.data.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
}

func (m *MDIO) mdioLow() {
	// RMII 0 pin level sets as output
	m.data.Low()
	m.data.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func (m *MDIO) sendBit(b bool) {
	m.mdioSet(b)
	m.delay()
	m.clk.High()
	m.delay()
	m.clk.Low()
}

func (m *MDIO) getBit() bool {
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
