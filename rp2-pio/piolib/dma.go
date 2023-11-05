//go:build rp2040

package piolib

import (
	"device/rp"
	"runtime/volatile"
	"unsafe"
)

type dmaChannel struct {
	hw      *dmaChannelHW
	channel uint8
}

// Single DMA channel. See rp.DMA_Type.
type dmaChannelHW struct {
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
var dmaChannels = (*[12]dmaChannelHW)(unsafe.Pointer(rp.DMA))

func getDMAChannel(channel uint8) *dmaChannel {
	return &dmaChannel{hw: &dmaChannels[channel], channel: channel}
}

const (
	_DREQ_PIO0_TX0   = 0x0
	_DREQ_PIO0_TX1   = 0x1
	_DREQ_PIO0_TX2   = 0x2
	_DREQ_PIO0_TX3   = 0x3
	_DREQ_PIO0_RX0   = 0x4
	_DREQ_PIO0_RX1   = 0x5
	_DREQ_PIO0_RX2   = 0x6
	_DREQ_PIO0_RX3   = 0x7
	_DREQ_PIO1_TX0   = 0x8
	_DREQ_PIO1_TX1   = 0x9
	_DREQ_PIO1_TX2   = 0xa
	_DREQ_PIO1_TX3   = 0xb
	_DREQ_PIO1_RX0   = 0xc
	_DREQ_PIO1_RX1   = 0xd
	_DREQ_PIO1_RX2   = 0xe
	_DREQ_PIO1_RX3   = 0xf
	_DREQ_SPI0_TX    = 0x10
	_DREQ_SPI0_RX    = 0x11
	_DREQ_SPI1_TX    = 0x12
	_DREQ_SPI1_RX    = 0x13
	_DREQ_UART0_TX   = 0x14
	_DREQ_UART0_RX   = 0x15
	_DREQ_UART1_TX   = 0x16
	_DREQ_UART1_RX   = 0x17
	_DREQ_PWM_WRAP0  = 0x18
	_DREQ_PWM_WRAP1  = 0x19
	_DREQ_PWM_WRAP2  = 0x1a
	_DREQ_PWM_WRAP3  = 0x1b
	_DREQ_PWM_WRAP4  = 0x1c
	_DREQ_PWM_WRAP5  = 0x1d
	_DREQ_PWM_WRAP6  = 0x1e
	_DREQ_PWM_WRAP7  = 0x1f
	_DREQ_I2C0_TX    = 0x20
	_DREQ_I2C0_RX    = 0x21
	_DREQ_I2C1_TX    = 0x22
	_DREQ_I2C1_RX    = 0x23
	_DREQ_ADC        = 0x24
	_DREQ_XIP_STREAM = 0x25
	_DREQ_XIP_SSITX  = 0x26
	_DREQ_XIP_SSIRX  = 0x27
)

type dmaTxSize uint32

const (
	dmaTxSize8 dmaTxSize = iota
	dmaTxSize16
	dmaTxSize32
)

type dmaChannelConfig struct {
	CTRL uint32
}

func getDefaultDMAConfig(channel uint32) (cc dmaChannelConfig) {
	cc.setRing(false, 0)
	cc.setBSwap(false)
	cc.setIRQQuiet(false)
	cc.setWriteIncrement(false)
	cc.setSniffEnable(false)
	cc.setHighPriority(false)

	cc.setChainTo(channel)
	cc.setTREQ_SEL(rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_PERMANENT)
	cc.setReadIncrement(true)
	cc.setTransferDataSize(dmaTxSize32)
	cc.setEnable(true)
	return cc
}

// push32 writes each element of src slice into the memory location at dst.
func (ch *dmaChannel) push32(dst *uint32, src []uint32, dreq uint32) {
	hw := ch.hw
	srcPtr := uint32(uintptr(unsafe.Pointer(&src[0])))
	dstPtr := uint32(uintptr(unsafe.Pointer(dst)))
	hw.READ_ADDR.Set(srcPtr)
	hw.WRITE_ADDR.Set(dstPtr)
	hw.TRANS_COUNT.Set(uint32(len(src)))
	// memfence
	var cc dmaChannelConfig
	cc.CTRL = hw.CTRL_TRIG.Get()
	cc.setTREQ_SEL(dreq)
	cc.setTransferDataSize(dmaTxSize32)
	cc.setChainTo(uint32(ch.channel))
	cc.setReadIncrement(true)
	cc.setWriteIncrement(false)
	cc.setEnable(true)

	hw.CTRL_TRIG.Set(cc.CTRL)

	retries := timeoutRetries
	for ch.busy() && retries > 0 {
		gosched()
		retries--
	}
	if retries == 0 {
		println("DMA push32 timeout")
	}
}

// pull32 reads the memory location at src into dst slice, incrementing dst pointer but not src.
func (ch *dmaChannel) pull32(dst []uint32, src *uint32, dreq uint32) {
	hw := ch.hw
	srcPtr := uint32(uintptr(unsafe.Pointer(src)))
	dstPtr := uint32(uintptr(unsafe.Pointer(&dst[0])))
	hw.READ_ADDR.Set(srcPtr)
	hw.WRITE_ADDR.Set(dstPtr)
	hw.TRANS_COUNT.Set(uint32(len(dst)))
	// memfence
	var cc dmaChannelConfig
	cc.CTRL = hw.CTRL_TRIG.Get()
	cc.setTREQ_SEL(dreq)
	cc.setTransferDataSize(dmaTxSize32)
	cc.setChainTo(uint32(ch.channel))
	cc.setReadIncrement(false)
	cc.setWriteIncrement(true)
	cc.setEnable(true)

	hw.CTRL_TRIG.Set(cc.CTRL)

	retries := timeoutRetries
	for ch.busy() && retries > 0 {
		gosched()
		retries--
	}
	if retries == 0 {
		println("DMA push32 timeout")
	}
}

// abort aborts the current transfer sequence on the channel and blocks until
// all in-flight transfers have been flushed through the address and data FIFOs.
// After this, it is safe to restart the channel.
func (ch *dmaChannel) abort() {
	// Each bit corresponds to a channel. Writing a 1 aborts whatever transfer
	// sequence is in progress on that channel. The bit will remain high until
	// any in-flight transfers have been flushed through the address and data FIFOs.
	// After writing, this register must be polled until it returns all-zero.
	// Until this point, it is unsafe to restart the channel.
	chMask := uint32(1 << ch.channel)
	rp.DMA.CHAN_ABORT.Set(chMask)
	retries := timeoutRetries
	for rp.DMA.CHAN_ABORT.Get()&chMask != 0 && retries > 0 {
		gosched()
		retries--
	}
	if retries == 0 {
		println("DMA abort timeout")
	}
}

func (ch *dmaChannel) busy() bool {
	return ch.hw.CTRL_TRIG.Get()&rp.DMA_CH0_CTRL_TRIG_BUSY != 0
}

// Select a Transfer Request signal. The channel uses the transfer request signal
// to pace its data transfer rate. Sources for TREQ signals are internal (TIMERS)
// or external (DREQ, a Data Request from the system). 0x0 to 0x3a -> select DREQ n as TREQ
func (cc *dmaChannelConfig) setTREQ_SEL(dreq uint32) {
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Msk)) | (uint32(dreq) << rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Pos)
}

func (cc *dmaChannelConfig) setChainTo(chainTo uint32) {
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Msk)) | (chainTo << rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Pos)
}

func (cc *dmaChannelConfig) setTransferDataSize(size dmaTxSize) {
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_DATA_SIZE_Msk)) | (uint32(size) << rp.DMA_CH0_CTRL_TRIG_DATA_SIZE_Pos)
}

func (cc *dmaChannelConfig) setRing(write bool, sizeBits uint32) {
	/*
		static inline void channel_config_set_ring(dma_channel_config *c, bool write, uint size_bits) {
		    assert(size_bits < 32);
		    c->ctrl = (c->ctrl & ~(DMA_CH0_CTRL_TRIG_RING_SIZE_BITS | DMA_CH0_CTRL_TRIG_RING_SEL_BITS)) |
		              (size_bits << DMA_CH0_CTRL_TRIG_RING_SIZE_LSB) |
		              (write ? DMA_CH0_CTRL_TRIG_RING_SEL_BITS : 0);
		}
	*/
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Msk)) |
		(sizeBits << rp.DMA_CH0_CTRL_TRIG_RING_SIZE_Pos)
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_RING_SEL_Pos, write)
}

func (cc *dmaChannelConfig) setReadIncrement(incr bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_INCR_READ_Pos, incr)
}

func (cc *dmaChannelConfig) setWriteIncrement(incr bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_INCR_WRITE_Pos, incr)
}

func (cc *dmaChannelConfig) setBSwap(bswap bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_BSWAP_Pos, bswap)
}

func (cc *dmaChannelConfig) setIRQQuiet(irqQuiet bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_IRQ_QUIET_Pos, irqQuiet)
}

func (cc *dmaChannelConfig) setHighPriority(highPriority bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_HIGH_PRIORITY_Pos, highPriority)
}

func (cc *dmaChannelConfig) setEnable(enable bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_EN_Pos, enable)
}

func (cc *dmaChannelConfig) setSniffEnable(sniffEnable bool) {
	setBitPos(&cc.CTRL, rp.DMA_CH0_CTRL_TRIG_SNIFF_EN_Pos, sniffEnable)
}

func setBitPos(cc *uint32, pos uint32, bit bool) {
	if bit {
		*cc = *cc | (1 << pos)
	} else {
		*cc = *cc & ^(1 << pos) // unset bit.
	}
}
