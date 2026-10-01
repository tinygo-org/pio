package piolib

import (
	"errors"
	"math"
	"runtime"
	"time"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

const timeoutRetries = math.MaxUint16 * 8

var (
	errTimeout           = errors.New("piolib:timeout")
	errContentionTimeout = errors.New("piolib:contention timeout")
	errBusy              = errors.New("piolib:busy")

	errDMAUnavail = errors.New("piolib:DMA channel unavailable")

	// errAsyncRequiresDMA is returned by asynchronous Tx helpers when DMA is
	// not enabled, since there is no non-blocking way to feed the TX FIFO
	// from software alone.
	errAsyncRequiresDMA = errors.New("piolib:async transfer requires DMA to be enabled")
)

//go:generate pioasm -o go parallel8.pio         parallel8_pio.go
//go:generate pioasm -o go pulsar.pio            pulsar_pio.go
//go:generate pioasm -o go spi.pio               spi_pio.go
//go:generate pioasm -o go ws2812b.pio           ws2812b_pio.go
//go:generate pioasm -o go i2s.pio               i2s_pio.go
//go:generate pioasm -o go spi3w.pio             spi3w_pio.go
//go:generate pioasm -o go ws2812bfourpixels.pio ws2812bfourpixels_pio.go

func gosched() {
	runtime.Gosched()
}

type deadline struct {
	t time.Time
}

func (dl deadline) expired() bool {
	if dl.t.IsZero() {
		return false
	}
	return time.Since(dl.t) > 0
}

type deadliner struct {
	// timeout is a bitshift value for the timeout.
	timeout uint8
}

func (ch deadliner) newDeadline() deadline {
	var t time.Time
	if ch.timeout != 0 {
		calc := time.Duration(1 << ch.timeout)
		t = time.Now().Add(calc)
	}
	return deadline{t: t}
}

func (ch *deadliner) setTimeout(timeout time.Duration) {
	if timeout <= 0 {
		ch.timeout = 0
		return // No timeout.
	}
	for i := uint8(0); i < 64; i++ {
		calc := time.Duration(1 << i)
		if calc > timeout {
			ch.timeout = i
			return
		}
	}
}

// helperPushUntilStall pushes buf data elements into TxReg through DMA if enabled or via [pio.StateMachine.TxPut] if dma disabled.
// It blocks until TxStall flag is set in state machine FDEBUG register. TxStall flag cleared immediately on this function call.
func helperPushUntilStall[T uint8 | uint16 | uint32](sm pio.StateMachine, dma dmaChannel, buf []T) (err error) {
	sm.ClearTxStalled()
	if dma.helperIsEnabled() {
		dreq := dmaPIO_TxDREQ(sm)
		err = dmaPush(dma, (*T)(unsafe.Pointer(sm.TxReg())), buf, dreq)
	} else {
		i := 0
		for i < len(buf) {
			if sm.IsTxFIFOFull() {
				gosched()
				continue
			}
			sm.TxPut(uint32(buf[i]))
			i++
		}
	}
	if err != nil {
		return err
	}
	for !sm.HasTxStalled() {
		gosched() // Block until empty.
	}
	return nil
}

// helperPushStart begins an asynchronous, DMA-backed push of buf into the
// state machine's TX FIFO and returns immediately, without waiting for the
// transfer to complete. DMA must already be enabled on dma (see
// [dmaChannel.helperEnableDMA]); errAsyncRequiresDMA is returned otherwise.
// It is the caller's responsibility to ensure no other transfer is already
// in flight on sm/dma before calling helperPushStart; see [helperPushBusy].
//
// buf must not be modified, reused for another transfer, or allowed to go out
// of scope until the transfer completes (see [helperPushBusy] and
// [helperPushWait]), since the DMA engine reads directly from its backing
// array in the background.
func helperPushStart[T uint8 | uint16 | uint32](sm pio.StateMachine, dma dmaChannel, buf []T) error {
	if !dma.helperIsEnabled() {
		return errAsyncRequiresDMA
	}
	if len(buf) == 0 {
		return nil // Nothing to do; TX-stalled flag already reads true.
	}
	sm.ClearTxStalled()
	dreq := dmaPIO_TxDREQ(sm)
	return dmaPushStart(dma, (*T)(unsafe.Pointer(sm.TxReg())), buf, dreq)
}

// helperPushBusy reports whether an asynchronous transfer started by
// [helperPushStart] is still in progress. Completion requires both the DMA
// channel to have finished moving data and the state machine to have drained
// its TX FIFO out to the pins (TX-stall), since the two can complete a few
// PIO cycles apart.
func helperPushBusy(sm pio.StateMachine, dma dmaChannel) bool {
	return dma.busy() || !sm.HasTxStalled()
}

// helperPushWait blocks until an asynchronous transfer started by
// [helperPushStart] has fully completed, i.e. [helperPushBusy] returns false.
// It is safe to call even if no asynchronous transfer is pending.
func helperPushWait(sm pio.StateMachine, dma dmaChannel) {
	for helperPushBusy(sm, dma) {
		gosched()
	}
}
