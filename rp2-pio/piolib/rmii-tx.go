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
	RefClk machine.Pin
}

type RMIITx struct {
	tx     pio.StateMachine
	txOff  uint8
	refclk machine.Pin
	dma    dmaChannel
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
	} else if cfg.RefClk == 0 {
		return errors.New("refclk cannot be GP0")
	}
	whole, frac, err := pio.ClkDivFromFrequency(cfg.Baud, machine.CPUFrequency())
	if err != nil {
		return err
	}
	// set to 1 to enable txen on SIDESET instead of SET.
	// set to 0 uses TXEN as SET pin.
	const sideTXEN = 1

	const (
		idxTx0 = iota
		idxTx1
		idxTxEN

		mskTx0  = 1 << idxTx0
		mskTx1  = 1 << idxTx1
		mskTXEN = (1 << idxTxEN) * (1 - sideTXEN)

		labelPreambleData = 2
		labelTxDeassert   = labelPreambleData + 4
		labelTxIdle       = labelTxDeassert + 2
		polRising         = true
	)

	// Program requires X set to amount of dibits to transmit during TXEN section.
	// X and Y will be zero on frame transmit success.
	// First 8 bytes are Preamble+SFD dibits.
	asm := pio.AssemblerV0{SidesetBits: sideTXEN}
	var txprog = [...]uint16{
		// Copyright (c) 2026 Patricio Whittingslow
		asm.Pull(false, true).Side(0).Encode(),
		asm.Set(pio.SetDestPins, mskTXEN).Side(0).Encode(),
		// asm.WaitPin(polRising, 0).Encode(),
		labelPreambleData:// Preamble+SFD+Data. TXEN asserted synchronous to first dibit.
		asm.Out(pio.OutDestPins, 2).Side(sideTXEN).Encode(),
		asm.Jmp(pio.JmpXNZeroDec, labelPreambleData).Side(sideTXEN).Encode(),

		// Why does a little more TXEN time yield better results?
		asm.Set(pio.SetDestPins, 0).Side(sideTXEN).Encode(),
		// Send inter-packet-gap(IPG) with TXEN deasserted.
		labelTxDeassert:// Deassertion of first 32 dibits=4 bytes.
		asm.Nop().Side(0).Encode(),
		asm.Jmp(pio.JmpYNZeroDec, labelTxDeassert).Side(0).Encode(),
		// .wrap_target
		labelTxIdle:// No data to send loop.
		asm.Nop().Side(0).Encode(),
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
	// cfg.RefClk.Configure(pinCfg)
	// Create state machine configuration.
	txcfg := asm.DefaultStateMachineConfig(txoff, txprog[:])
	// txcfg.SetInPins(cfg.RefClk, 1)
	txcfg.SetWrap(txoff+labelTxIdle, txoff+uint8(len(txprog))-1)
	txcfg.SetOutPins(cfg.TxBase, 2)          // OUT pins: TX0,TX1
	txcfg.SetSetPins(cfg.TxBase, 3-sideTXEN) // SET pins: TX0,TX1,TXEN
	if sideTXEN == 1 {
		txcfg.SetSidesetPins(cfg.TxBase + idxTxEN)
	}
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
	r.refclk = cfg.RefClk
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
