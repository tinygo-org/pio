package main

import (
	"device/rp"
	"runtime/volatile"
	"unsafe"
)

// Single DMA channel. See rp.DMA_Type.
type dmaChannel struct {
	READ_ADDR   volatile.Register32
	WRITE_ADDR  volatile.Register32
	TRANS_COUNT volatile.Register32
	CTRL_TRIG   volatile.Register32
	_           [12]volatile.Register32 // aliases
}

// Static assignment of DMA channels to peripherals.
// Allocating them statically is good enough for now. If lots of peripherals use
// DMA, these might need to be assigned at runtime.
const (
	spi0DMAChannel = iota
	spi1DMAChannel
)

// DMA channels usable on the RP2040.
var dmaChannels = (*[12]dmaChannel)(unsafe.Pointer(rp.DMA))

const (
	DREQ_PIO0_TX0   = 0x0
	DREQ_PIO0_TX1   = 0x1
	DREQ_PIO0_TX2   = 0x2
	DREQ_PIO0_TX3   = 0x3
	DREQ_PIO0_RX0   = 0x4
	DREQ_PIO0_RX1   = 0x5
	DREQ_PIO0_RX2   = 0x6
	DREQ_PIO0_RX3   = 0x7
	DREQ_PIO1_TX0   = 0x8
	DREQ_PIO1_TX1   = 0x9
	DREQ_PIO1_TX2   = 0xa
	DREQ_PIO1_TX3   = 0xb
	DREQ_PIO1_RX0   = 0xc
	DREQ_PIO1_RX1   = 0xd
	DREQ_PIO1_RX2   = 0xe
	DREQ_PIO1_RX3   = 0xf
	DREQ_SPI0_TX    = 0x10
	DREQ_SPI0_RX    = 0x11
	DREQ_SPI1_TX    = 0x12
	DREQ_SPI1_RX    = 0x13
	DREQ_UART0_TX   = 0x14
	DREQ_UART0_RX   = 0x15
	DREQ_UART1_TX   = 0x16
	DREQ_UART1_RX   = 0x17
	DREQ_PWM_WRAP0  = 0x18
	DREQ_PWM_WRAP1  = 0x19
	DREQ_PWM_WRAP2  = 0x1a
	DREQ_PWM_WRAP3  = 0x1b
	DREQ_PWM_WRAP4  = 0x1c
	DREQ_PWM_WRAP5  = 0x1d
	DREQ_PWM_WRAP6  = 0x1e
	DREQ_PWM_WRAP7  = 0x1f
	DREQ_I2C0_TX    = 0x20
	DREQ_I2C0_RX    = 0x21
	DREQ_I2C1_TX    = 0x22
	DREQ_I2C1_RX    = 0x23
	DREQ_ADC        = 0x24
	DREQ_XIP_STREAM = 0x25
	DREQ_XIP_SSITX  = 0x26
	DREQ_XIP_SSIRX  = 0x27
)

type DMATransferSize uint32

const (
	DMA_SIZE_8 DMATransferSize = iota
	DMA_SIZE_16
	DMA_SIZE_32
)

/*
	static inline dma_channel_config dma_channel_get_default_config(uint channel) {
	    dma_channel_config c = {0};
	    channel_config_set_read_increment(&c, true);
	    channel_config_set_write_increment(&c, false);
	    channel_config_set_dreq(&c, DREQ_FORCE);
	    channel_config_set_chain_to(&c, channel);
	    channel_config_set_transfer_data_size(&c, DMA_SIZE_32);
	    channel_config_set_ring(&c, false, 0);
	    channel_config_set_bswap(&c, false);
	    channel_config_set_irq_quiet(&c, false);
	    channel_config_set_enable(&c, true);
	    channel_config_set_sniff_enable(&c, false);
	    channel_config_set_high_priority( &c, false);
	    return c;
	}
*/
func getDefaultDMAConfig(channel uint32) uint32 {
	var cc uint32
	setReadIncrement(cc, true)
	setWriteIncrement(cc, false)
	setDREQ(cc, rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_PERMANENT)
	setChainTo(cc, channel)
	setTransferDataSize(cc, DMA_SIZE_32)
	setRing(cc, false, 0)
	setBSwap(cc, false)
	setIRQQuiet(cc, false)
	setEnable(cc, true)
	setSniffEnable(cc, false)
	setHighPriority(cc, false)
	return cc
}

/*
	static inline void channel_config_set_read_increment(dma_channel_config *c, bool incr) {
	    c->ctrl = incr ? (c->ctrl | DMA_CH0_CTRL_TRIG_INCR_READ_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_INCR_READ_BITS);
	}
*/
func setReadIncrement(cc uint32, incr bool) {
	if incr {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_INCR_READ
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_INCR_READ)
}

/*
	static inline void channel_config_set_write_increment(dma_channel_config *c, bool incr) {
	    c->ctrl = incr ? (c->ctrl | DMA_CH0_CTRL_TRIG_INCR_WRITE_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_INCR_WRITE_BITS);
	}
*/
func setWriteIncrement(cc uint32, incr bool) {
	if incr {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_INCR_WRITE
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_INCR_WRITE)
}

/*
	static inline void channel_config_set_dreq(dma_channel_config *c, uint dreq) {
	    assert(dreq <= DREQ_FORCE);
	    c->ctrl = (c->ctrl & ~DMA_CH0_CTRL_TRIG_TREQ_SEL_BITS) | (dreq << DMA_CH0_CTRL_TRIG_TREQ_SEL_LSB);
	}
*/
func setDREQ(cc uint32, dreq uint32) {
	cc = (cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Msk)) | (uint32(dreq) << rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Pos)
}

/*
	static inline void channel_config_set_chain_to(dma_channel_config *c, uint chain_to) {
	    assert(chain_to <= NUM_DMA_CHANNELS);
	    c->ctrl = (c->ctrl & ~DMA_CH0_CTRL_TRIG_CHAIN_TO_BITS) | (chain_to << DMA_CH0_CTRL_TRIG_CHAIN_TO_LSB);
	}
*/
func setChainTo(cc uint32, chainTo uint32) {
	cc = (cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Msk)) | (chainTo << rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Pos)
}

/*
	static inline void channel_config_set_transfer_data_size(dma_channel_config *c, enum dma_channel_transfer_size size) {
	    assert(size == DMA_SIZE_8 || size == DMA_SIZE_16 || size == DMA_SIZE_32);
	    c->ctrl = (c->ctrl & ~DMA_CH0_CTRL_TRIG_DATA_SIZE_BITS) | (((uint)size) << DMA_CH0_CTRL_TRIG_DATA_SIZE_LSB);
	}
*/
func setTransferDataSize(cc uint32, size DMATransferSize) {
	cc = (cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_DATA_SIZE_Msk)) | (uint32(size) << rp.DMA_CH0_CTRL_TRIG_DATA_SIZE_Pos)
}

/*
	static inline void channel_config_set_ring(dma_channel_config *c, bool write, uint size_bits) {
	    assert(size_bits < 32);
	    c->ctrl = (c->ctrl & ~(DMA_CH0_CTRL_TRIG_RING_SIZE_BITS | DMA_CH0_CTRL_TRIG_RING_SEL_BITS)) |
	              (size_bits << DMA_CH0_CTRL_TRIG_RING_SIZE_LSB) |
	              (write ? DMA_CH0_CTRL_TRIG_RING_SEL_BITS : 0);
	}
*/
func setRing(cc uint32, write bool, sizeBits uint32) {
	if write {
		cc = (cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Msk|rp.DMA_CH0_CTRL_TRIG_RING_SEL_Pos)) |
			(sizeBits << rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Pos) | rp.DMA_CH0_CTRL_TRIG_RING_SEL_Pos
		return
	}
	cc = (cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Msk|rp.DMA_CH0_CTRL_TRIG_RING_SEL_Pos)) |
		(sizeBits << rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Pos) | 0
}

/*
	static inline void channel_config_set_bswap(dma_channel_config *c, bool bswap) {
	    c->ctrl = bswap ? (c->ctrl | DMA_CH0_CTRL_TRIG_BSWAP_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_BSWAP_BITS);
	}
*/
func setBSwap(cc uint32, bswap bool) {
	if bswap {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_BSWAP_Pos
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_BSWAP_Pos)
}

/*
	static inline void channel_config_set_irq_quiet(dma_channel_config *c, bool irq_quiet) {
	    c->ctrl = irq_quiet ? (c->ctrl | DMA_CH0_CTRL_TRIG_IRQ_QUIET_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_IRQ_QUIET_BITS);
	}
*/
func setIRQQuiet(cc uint32, irqQuiet bool) {
	if irqQuiet {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_IRQ_QUIET_Pos
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_IRQ_QUIET_Pos)
}

/*
	static inline void channel_config_set_high_priority(dma_channel_config *c, bool high_priority) {
	    c->ctrl = high_priority ? (c->ctrl | DMA_CH0_CTRL_TRIG_HIGH_PRIORITY_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_HIGH_PRIORITY_BITS);
	}
*/
func setHighPriority(cc uint32, highPriority bool) {
	if highPriority {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_HIGH_PRIORITY_Pos
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_HIGH_PRIORITY_Pos)
}

/*
	static inline void channel_config_set_enable(dma_channel_config *c, bool enable) {
	    c->ctrl = enable ? (c->ctrl | DMA_CH0_CTRL_TRIG_EN_BITS) : (c->ctrl & ~DMA_CH0_CTRL_TRIG_EN_BITS);
	}
*/
func setEnable(cc uint32, enable bool) {
	if enable {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_EN_Pos
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_EN_Pos)
}

/*
	static inline void channel_config_set_sniff_enable(dma_channel_config *c, bool sniff_enable) {
	    c->ctrl = sniff_enable ? (c->ctrl | DMA_CH0_CTRL_TRIG_SNIFF_EN_BITS) : (c->ctrl &
	                                                                             ~DMA_CH0_CTRL_TRIG_SNIFF_EN_BITS);
	}
*/
func setSniffEnable(cc uint32, sniffEnable bool) {
	if sniffEnable {
		cc = cc | rp.DMA_CH0_CTRL_TRIG_SNIFF_EN_Pos
		return
	}
	cc = cc & ^uint32(rp.DMA_CH0_CTRL_TRIG_SNIFF_EN_Pos)
}
