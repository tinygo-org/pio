package piolib

import (
	"errors"
	"machine"
	"math"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type RMIITxConfig struct {
	// Baud is the transmit frequency. Set to 100_000_000 for 100M operation.
	Baud uint32
	// TxBuffer must be at least 1518 in size. MTU=1500 + EthernetFrame=14 + CRC=4
	TxBuffer []byte
	// TxBase is the first pin of the consecutive, ordered set [TX0,TX1,TXEN]
	TxBase machine.Pin
}

type RMIITx struct {
	tx    pio.StateMachine
	txOff uint8
	dma   dmaChannel
	// Buf must contain preamble+SFD+CRC+actual frame content.
	buf []byte
}

const preamblesfd = "\x55\x55\x55\x55\x55\x55\x55" + // Preamble.
	"\x57" // SFD.

func (r *RMIITx) Configure(PIO *pio.PIO, cfg RMIITxConfig) error {
	if len(cfg.TxBuffer) < 1518 {
		return errors.New("RMIITx buffer too short")
	} else if len(cfg.TxBuffer) > math.MaxUint16 {
		return errors.New("buffer too long")
	}
	whole, frac, err := pio.ClkDivFromFrequency(cfg.Baud, machine.CPUFrequency())
	if err != nil {
		return err
	}

	const (
		idxTx0 = iota
		idxTx1
		idxTxEN

		mskTx0            = 1 << idxTx0
		mskTx1            = 1 << idxTx1
		mskTXEN           = 1 << idxTxEN
		labelPreambleData = 2
		labelTxDeassert   = 5
		labelTxIdle       = 7
	)

	// Program requires X set to amount of dibits to transmit during TXEN section.
	// X and Y will be zero on frame transmit success.
	// First 8 bytes are Preamble+SFD dibits.
	asm := pio.AssemblerV0{SidesetBits: 0}
	var txprog = [...]uint16{
		// Copyright (c) 2026 Patricio Whittingslow
		asm.Pull(false, true).Encode(),
		asm.Set(pio.SetDestPins, mskTXEN).Encode(),
		labelPreambleData:// Preamble+SFD+Data. TXEN asserted synchronous to first dibit.
		asm.Out(pio.OutDestPins, 2).Encode(),
		asm.Jmp(pio.JmpXNZeroDec, labelPreambleData).Encode(),

		// Send inter-packet-gap(IPG) with TXEN deasserted.
		asm.Set(pio.SetDestPins, 0).Encode(),
		labelTxDeassert:// Deassertion of first 32 dibits=4 bytes.
		asm.Nop().Encode(),
		asm.Jmp(pio.JmpYNZeroDec, labelTxDeassert).Encode(),
		// .wrap_target
		labelTxIdle:// No data to send loop.
		asm.Nop().Encode(),
		// .wrap
	}
	txoff, err := PIO.AddProgram(txprog[:], -1)
	if err != nil {
		return err
	}
	r.txOff = txoff
	txSM, err := PIO.ClaimStateMachine()
	if err != nil {
		return err
	}
	pinCfg := machine.PinConfig{Mode: PIO.PinMode()}
	var txPinMsk uint32 = 0b111 << cfg.TxBase
	for i := machine.Pin(0); i < 3; i++ {
		pin := (cfg.TxBase + i)
		pin.Configure(pinCfg)
	}

	// Create state machine configuration.
	txcfg := asm.DefaultStateMachineConfig(txoff, txprog[:])
	txcfg.SetWrap(txoff+labelTxIdle, txoff+uint8(len(txprog))-1)
	txcfg.SetOutPins(cfg.TxBase, 2)  // OUT pins: TX0,TX1
	txcfg.SetSetPins(cfg.TxBase, 3)  // SET pins: TX0,TX1,TXEN
	txcfg.SetOutShift(true, true, 8) // LSB sent out first, must shift right.
	txcfg.SetClkDivIntFrac(whole, frac)
	txcfg.SetFIFOJoin(pio.FifoJoinTx)

	txSM.Init(txoff+labelTxIdle, txcfg)
	txSM.SetPindirsMasked(txPinMsk, txPinMsk)
	txSM.SetPinsMasked(0, txPinMsk) // Set bus to idle.
	txSM.SetX(0)
	txSM.SetEnabled(true)

	r.tx = txSM
	r.buf = cfg.TxBuffer
	r.enableDMA(true)
	return nil
}

func (r *RMIITx) enableDMA(b bool) {
	r.dma.helperEnableDMA(b)
}

func (r *RMIITx) isDMAEnabled() bool {
	return r.dma.helperIsEnabled()
}

func (r *RMIITx) bufbytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&r.buf[0])), len(r.buf)*4)
}

func (r *RMIITx) IsSending() bool {
	return r.tx.IsEnabled() && (r.dma.busy() || !r.tx.IsTxFIFOEmpty())
}

const preambleByte = 0b0101_0101

var preambleSFD = []byte{preambleByte, preambleByte, preambleByte, preambleByte,
	preambleByte, preambleByte, preambleByte, 0b1101_0101}

func (r *RMIITx) SendFrame(frame []byte) error {
	if r.dma.busy() {
		return errors.New("DMA busy")
	}
	buf := r.buf[:]
	n := copy(buf[:], preambleSFD)
	n += copy(buf[n:], frame)

	r.tx.SetEnabled(false)
	r.tx.ClearFIFOs()
	r.tx.Restart()
	r.tx.SetX(uint32(n)*4 - 1)
	r.tx.SetY(48 - 1)
	r.tx.Jmp(pio.JmpAlways, r.txOff)
	r.tx.SetEnabled(true)
	dreq := dmaPIO_TxDREQ(r.tx)
	err := r.dma.Push8((*byte)(unsafe.Pointer(&r.tx.TxReg().Reg)), buf[:n], dreq)
	if err != nil {
		return err
	}
	return nil
}
