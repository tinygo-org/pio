//go:build rp2040 || rp2350

package piolib

import (
	"device/rp"
	"runtime/volatile"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

var _DMA = &dmaArbiter{}

type dmaArbiter struct {
	claimedChannels uint16
}

// ClaimChannel returns a DMA channel that can be used for DMA transfers.
func (arb *dmaArbiter) ClaimChannel() (channel dmaChannel, ok bool) {
	for i := uint8(0); i < 12; i++ {
		ch := arb.Channel(i)
		if ch.TryClaim() {
			return ch, true
		}
	}
	return dmaChannel{}, false
}

func (arb *dmaArbiter) Channel(channel uint8) dmaChannel {
	if channel > 11 {
		panic("invalid DMA channel")
	}
	// DMA channels usable on the RP2040. 12 in total.
	var dmaChannels = (*[12]dmaChannelHW)(unsafe.Pointer(rp.DMA))
	return dmaChannel{
		hw:  &dmaChannels[channel],
		arb: arb,
		idx: channel,
	}
}

func (dma *dmaChannel) helperIsEnabled() bool {
	return dma.IsValid()
}

func (dma *dmaChannel) helperEnableDMA(enabled bool) error {
	dmaAlreadyEnabled := dma.helperIsEnabled()
	if !enabled || dmaAlreadyEnabled {
		if !enabled && dmaAlreadyEnabled {
			dma.Unclaim()
			*dma = dmaChannel{} // Invalidate DMA channel.
		}
		return nil
	}
	channel, ok := _DMA.ClaimChannel()
	if !ok {
		return errDMAUnavail
	}
	channel.dl = dma.dl // save deadliner from existing DMA channel, maybe set by user in future.
	*dma = channel
	return nil
}

type dmaChannel struct {
	hw  *dmaChannelHW
	arb *dmaArbiter
	dl  deadliner
	idx uint8
}

// TryClaim claims the DMA channel for use by a peripheral and returns if it succeeded in claiming the channel.
func (ch dmaChannel) TryClaim() bool {
	ch.mustValid()
	if ch.IsClaimed() {
		return false
	}
	ch.arb.claimedChannels |= 1 << ch.idx
	return true
}

// Unclaim releases the DMA channel so it can be used by other peripherals.
// It does not check if the channel is currently claimed; it force-unclaims the channel.
func (ch dmaChannel) Unclaim() {
	ch.mustValid()
	ch.arb.claimedChannels &^= 1 << ch.idx
}

// IsClaimed returns true if the DMA channel is currently claimed through software.
func (ch dmaChannel) IsClaimed() bool {
	ch.mustValid()
	return ch.arb.claimedChannels&(1<<ch.idx) != 0
}

// IsValid returns true if the DMA channel was created successfully.
func (ch dmaChannel) IsValid() bool {
	return ch.hw != nil && ch.arb == _DMA
}

// ChannelIndex returns the channel number of the DMA channel. In range 0..11.
func (ch dmaChannel) ChannelIndex() uint8 { return ch.idx }

// HW returns the hardware registers for this DMA channel.
func (ch dmaChannel) HW() *dmaChannelHW { return ch.hw }

func (ch dmaChannel) Init(cfg dmaChannelConfig) {
	ch.mustValid()
	ch.HW().CTRL_TRIG.Set(cfg.CTRL)
}

// CurrentConfig copies the actual configuration of the DMA channel.
func (ch dmaChannel) CurrentConfig() dmaChannelConfig {
	ch.mustValid()
	return dmaChannelConfig{CTRL: ch.HW().CTRL_TRIG.Get()}
}

func (ch dmaChannel) mustValid() {
	if !ch.IsValid() {
		panic("use of uninitialized DMA channel")
	}
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

// dmaPIO_TREQ returns the Tx DREQ signal for a PIO state machine.
func dmaPIO_TxDREQ(sm pio.StateMachine) uint32 {
	return _DREQ_PIO0_TX0 + uint32(sm.PIO().BlockIndex())*8 + uint32(sm.StateMachineIndex())
}

// dmaPIO_TREQ returns the Rx DREQ signal for a PIO state machine.
func dmaPIO_RxDREQ(sm pio.StateMachine) uint32 {
	return dmaPIO_TxDREQ(sm) + 4
}

// 2.5.3.1. System DREQ Table. Note: Another caveat is that multiple channels should not be connected to the same DREQ.
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

// Push32 writes each element of src slice into the memory location at dst.
func (ch dmaChannel) Push32(dst *uint32, src []uint32, dreq uint32) error {
	return dmaPush(ch, dst, src, dreq)
}

// Push16 writes each element of src slice into the memory location at dst.
func (ch dmaChannel) Push16(dst *uint16, src []uint16, dreq uint32) error {
	return dmaPush(ch, dst, src, dreq)
}

// Push8 writes each element of src slice into the memory location at dst.
func (ch dmaChannel) Push8(dst *byte, src []byte, dreq uint32) error {
	return dmaPush(ch, dst, src, dreq)
}

// Push32 writes each element of src slice into the memory location at dst.
func dmaPush[T uint8 | uint16 | uint32](ch dmaChannel, dst *T, src []T, dreq uint32) error {
	// If currently busy we wait until safe to edit hardware registers.
	deadline := ch.dl.newDeadline()
	for ch.busy() {
		if deadline.expired() {
			return errContentionTimeout
		}
		gosched()
	}

	hw := ch.HW()
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	srcPtr := uint32(uintptr(unsafe.Pointer(&src[0])))
	dstPtr := uint32(uintptr(unsafe.Pointer(dst)))
	hw.READ_ADDR.Set(srcPtr)
	hw.WRITE_ADDR.Set(dstPtr)
	hw.TRANS_COUNT.Set(uint32(len(src)))

	// memfence

	cc := ch.CurrentConfig()
	cc.setTREQ_SEL(dreq)
	cc.setTransferDataSize(dmaSize[T]())
	cc.setChainTo(ch.idx)
	cc.setReadIncrement(true)
	cc.setWriteIncrement(false)
	cc.setEnable(true)

	// We begin our DMA transfer here!
	hw.CTRL_TRIG.Set(cc.CTRL)

	deadline = ch.dl.newDeadline()
	for ch.busy() {
		if deadline.expired() {
			ch.abort()
			return errTimeout
		}
		gosched()
	}
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	return nil
}

// Pull32 reads the memory location at src into dst slice, incrementing dst pointer but not src.
func (ch dmaChannel) Pull32(dst []uint32, src *uint32, dreq uint32) error {
	return dmaPull(ch, dst, src, dreq)
}

// Pull16 reads the memory location at src into dst slice, incrementing dst pointer but not src.
func (ch dmaChannel) Pull16(dst []uint16, src *uint16, dreq uint32) error {
	return dmaPull(ch, dst, src, dreq)
}

// Pull8 reads the memory location at src into dst slice, incrementing dst pointer but not src.
func (ch dmaChannel) Pull8(dst []byte, src *byte, dreq uint32) error {
	return dmaPull(ch, dst, src, dreq)
}

// Pull32 reads the memory location at src into dst slice, incrementing dst pointer but not src.
func dmaPull[T uint8 | uint16 | uint32](ch dmaChannel, dst []T, src *T, dreq uint32) error {
	// If currently busy we wait until safe to edit hardware registers.
	deadline := ch.dl.newDeadline()
	for ch.busy() {
		if deadline.expired() {
			return errContentionTimeout
		}
		gosched()
	}

	hw := ch.HW()
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	srcPtr := uint32(uintptr(unsafe.Pointer(src)))
	dstPtr := uint32(uintptr(unsafe.Pointer(&dst[0])))
	hw.READ_ADDR.Set(srcPtr)
	hw.WRITE_ADDR.Set(dstPtr)
	hw.TRANS_COUNT.Set(uint32(len(dst)))

	// memfence

	cc := ch.CurrentConfig()
	cc.setTREQ_SEL(dreq)
	cc.setTransferDataSize(dmaSize[T]())
	cc.setChainTo(ch.idx)
	cc.setReadIncrement(false)
	cc.setWriteIncrement(true)
	cc.setEnable(true)

	// We begin our DMA transfer here!
	hw.CTRL_TRIG.Set(cc.CTRL)

	deadline = ch.dl.newDeadline()
	for ch.busy() {
		if deadline.expired() {
			ch.abort()
			return errTimeout
		}
		gosched()
	}
	return nil
}

func dmaSize[T uint8 | uint16 | uint32]() dmaTxSize {
	var a T
	switch unsafe.Sizeof(a) {
	case 1:
		return dmaTxSize8
	case 2:
		return dmaTxSize16
	case 4:
		return dmaTxSize32
	default:
		panic("invalid DMA transfer size")
	}
}

// abort aborts the current transfer sequence on the channel and blocks until
// all in-flight transfers have been flushed through the address and data FIFOs.
// After this, it is safe to restart the channel.
func (ch dmaChannel) abort() {
	// Each bit corresponds to a channel. Writing a 1 aborts whatever transfer
	// sequence is in progress on that channel. The bit will remain high until
	// any in-flight transfers have been flushed through the address and data FIFOs.
	// After writing, this register must be polled until it returns all-zero.
	// Until this point, it is unsafe to restart the channel.
	chMask := uint32(1 << ch.idx)
	rp.DMA.CHAN_ABORT.Set(chMask)

	deadline := ch.dl.newDeadline()
	for rp.DMA.CHAN_ABORT.Get()&chMask != 0 {
		if deadline.expired() {
			println("DMA abort timeout")
			break
		}
		gosched()
	}
}

func (ch dmaChannel) busy() bool {
	hw := ch.HW()
	return hw.CTRL_TRIG.Get()&rp.DMA_CH0_CTRL_TRIG_BUSY != 0
}

type dmaTxSize uint32

const (
	dmaTxSize8 dmaTxSize = iota
	dmaTxSize16
	dmaTxSize32
)

type dmaChannelConfig struct {
	CTRL uint32
}

func dmaDefaultConfig(channel uint8) (cc dmaChannelConfig) {
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
	// cc.setEnable(true)
	return cc
}

// Select a Transfer Request signal. The channel uses the transfer request signal
// to pace its data transfer rate. Sources for TREQ signals are internal (TIMERS)
// or external (DREQ, a Data Request from the system). 0x0 to 0x3a -> select DREQ n as TREQ
func (cc *dmaChannelConfig) setTREQ_SEL(dreq uint32) {
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Msk)) | (uint32(dreq) << rp.DMA_CH0_CTRL_TRIG_TREQ_SEL_Pos)
}

func (cc *dmaChannelConfig) setChainTo(chainTo uint8) {
	cc.CTRL = (cc.CTRL & ^uint32(rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Msk)) | (uint32(chainTo) << rp.DMA_CH0_CTRL_TRIG_CHAIN_TO_Pos)
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

func ptrAs[T ~uint32](ptr *T) uint32 {
	return uint32(uintptr(unsafe.Pointer(ptr)))
}
