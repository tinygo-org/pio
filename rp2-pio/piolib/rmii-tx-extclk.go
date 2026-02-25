package piolib

import (
	"errors"
	"machine"
	"math"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

/*
Reference PIO assembly from rscott (runs at full system clock with timing delays):

.program rmii_ethernet_phy_tx_data_gp
.wrap_target
public tx_start:
pull block [TX_BYTE-2]
wait 1 pin 0
wait 0 pin 0 [TX_D0]
set pins, 0b101 [TX_P0]
out x, 8 [TX_P0]
in x, 8 [TX_P0]
out x, 8 [TX_P0]
in x, 24 [TX_P0]
mov y, ISR [TX_P1]
set pins, 0b111 [TX_DIBIT]
out pins, 2 [TX_DIBIT-1]
jmp y--, xmit
set pins, 0b000 [TX_BYTE-1]
set x, 9
jmp x--, ipg [TX_BYTE]
.wrap
*/

// RMIITxConfig configures a PIO-based RMII transmitter.
type RMIITxConfig struct {
	// Baud is the transmit frequency. Set to 100_000_000 for 100M operation.
	Baud uint32
	// TxBuffer must be at least 1518 in size. MTU=1500 + EthernetFrame=14 + CRC=4
	TxBuffer []byte
	// TxBase is the first pin of the consecutive, ordered set [TX0,TX1,TXEN]
	TxBase machine.Pin
	RefClk machine.Pin
}

// RMIITxExtClk is a RMII transmitter that synchronizes to an external 50 MHz
// reference clock provided by the PHY. Unlike [rmiiTx], which free-runs the PIO
// at the RMII rate, RMIITxExtClk runs the PIO state machine at full CPU clock
// and aligns dibit outputs to the falling edge of RefClk using wait instructions
// with computed delays. Preamble, SFD, and IPG are generated internally by PIO.
//
// The CPU frequency must be a multiple of 50 MHz in the range 100–300 MHz.
// Based on rscott's timed RMII transmit design.
type RMIITxExtClk struct {
	tx     pio.StateMachine
	txOff  uint8
	refclk machine.Pin
	dma    dmaChannel
	buf    []byte
}

// Configure sets up the PIO state machine and DMA for RMII transmission.
// The Baud field in cfg is ignored; PIO runs at the CPU clock frequency.
// TxBuffer must be at least 1520 bytes (2 count bytes + max Ethernet frame).
func (r *RMIITxExtClk) Configure(PIO *pio.PIO, cfg RMIITxConfig) error {
	if len(cfg.TxBuffer) < 1520 {
		return errors.New("RMIITxTimed buffer too short")
	} else if len(cfg.TxBuffer) > math.MaxUint16 {
		return errors.New("buffer too long")
	} else if cfg.RefClk == 0 || cfg.RefClk == machine.NoPin {
		return errors.New("need a clock that is not GP0")
	}

	// PIO runs at full CPU clock. Compute timing from CPU frequency and 50 MHz RMII clock.
	const rmiiClk = 50_000_000 // 50 MHz RMII dibit clock.
	cpuFreq := machine.CPUFrequency()
	if cpuFreq%rmiiClk != 0 {
		return errors.New("CPU frequency must be a multiple of 50 MHz")
	}
	txPhase := cpuFreq / rmiiClk // PIO cycles per RMII dibit period.
	if txPhase < 2 || txPhase > 6 {
		return errors.New("CPU frequency out of range (need 100-300 MHz)")
	}

	txDibit := txPhase - 1
	txByte := txPhase*4 - 1

	// Preamble: 6 instructions span 31 dibit periods = 31*txPhase cycles.
	ptot := 32 * txPhase
	pd1 := ptot / 6
	pd2 := (ptot - txPhase) - 5*pd1
	txP0 := pd1 - 1
	txP1 := pd2 - 1

	// Falling-edge alignment delay.
	cpuMHz := cpuFreq / 1_000_000
	txD0 := (cpuMHz / 150) + (cpuMHz / 200) - ((cpuMHz / 300) * 2)

	const (
		idxTx0 = iota
		idxTx1
		idxTxEN

		labelTxStart = 0
		labelPreamb  = 3
		labelXmit    = 10
		labelIPG     = 14
	)

	// Copyright (c) 2026 Patricio Whittingslow. PIO program based on rscott's design.
	asm := pio.AssemblerV0{SidesetBits: 0}
	var txprog = [...]uint16{
		// .wrap_target
		asm.Pull(false, true).Delay(uint8(txByte - 2)).Encode(), // IPG byte time.
		asm.WaitPin(true, 0).Encode(),                           // Wait for RMII clock rising edge.
		asm.WaitPin(false, 0).Delay(uint8(txD0)).Encode(),       // Wait for falling edge + alignment.

		labelPreamb:// Preamble: hold TX=01, TXEN=1 for 31 dibit periods.
		asm.Set(pio.SetDestPins, 0b101).Delay(uint8(txP0)).Encode(), // TXEN=1, TX=01.
		asm.Out(pio.OutDestX, 8).Delay(uint8(txP0)).Encode(),             // Get dibit count LSB.
		asm.In(pio.InSrcX, 8).Delay(uint8(txP0)).Encode(),                // Accumulate into ISR.
		asm.Out(pio.OutDestX, 8).Delay(uint8(txP0)).Encode(),             // Get dibit count MSB.
		asm.In(pio.InSrcX, 24).Delay(uint8(txP0)).Encode(),               // Merge MSB with LSB.
		asm.Mov(pio.MovDestY, pio.MovSrcISR).Delay(uint8(txP1)).Encode(), // Y = dibit count - 1.

		// SFD last dibit: TX=11, TXEN=1.
		asm.Set(pio.SetDestPins, 0b111).Delay(uint8(txDibit)).Encode(),

		labelXmit:// Transmit frame data dibits.
		asm.Out(pio.OutDestPins, 2).Delay(uint8(txDibit - 1)).Encode(),
		asm.Jmp(pio.JmpYNZeroDec, labelXmit).Encode(),

		// Idle bus and Inter-Packet Gap (12 byte times total).
		asm.Set(pio.SetDestPins, 0b000).Delay(uint8(txByte - 1)).Encode(), // 1 byte.
		asm.Set(pio.SetDestX, 9).Encode(),                                 // 10-iteration loop.
		labelIPG:                                                          asm.Jmp(pio.JmpXNZeroDec, labelIPG).Delay(uint8(txByte)).Encode(), // 10 bytes.
		// .wrap → back to pull block (1 byte).
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
	txcfg := asm.DefaultStateMachineConfig(txoff, txprog[:])
	txcfg.SetOutPins(cfg.TxBase, 2)  // OUT pins: TX0,TX1.
	txcfg.SetSetPins(cfg.TxBase, 3)  // SET pins: TX0,TX1,TXEN.
	txcfg.SetInPins(cfg.RefClk, 1)   // IN pins: RefClk (for wait).
	txcfg.SetOutShift(true, true, 8) // Right shift, autopull at 8 bits.
	txcfg.SetClkDivIntFrac(1, 0)     // Run at CPU clock.
	txcfg.SetFIFOJoin(pio.FifoJoinTx)

	txSM.Init(txoff+labelTxStart, txcfg)
	txSM.SetPindirsMasked(txPinMsk, txPinMsk)
	txSM.SetPinsMasked(0, txPinMsk) // Set bus to idle.
	txSM.SetEnabled(true)

	r.tx = txSM
	r.buf = cfg.TxBuffer
	r.refclk = cfg.RefClk
	r.enableDMA(true)
	return nil
}

func (r *RMIITxExtClk) enableDMA(b bool) {
	r.dma.helperEnableDMA(b)
}

func (r *RMIITxExtClk) isDMAEnabled() bool {
	return r.dma.helperIsEnabled()
}

// IsSending returns true if a frame transmission is still in progress.
func (r *RMIITxExtClk) IsSending() bool {
	return r.tx.IsEnabled() && (r.dma.busy() || !r.tx.IsTxFIFOEmpty())
}

// SendFrame transmits an Ethernet frame over RMII. The frame must include
// the Ethernet header (14 bytes) and CRC (4 bytes). Preamble and SFD are
// generated by the PIO program. The first two DMA bytes encode the dibit
// count for the PIO program's transmit loop.
func (r *RMIITxExtClk) SendFrame(frame []byte) error {
	if r.dma.busy() {
		return errors.New("DMA busy")
	}
	buf := r.buf[:]
	// DMA data: [countLSB, countMSB, frame...]
	dibitCount := uint16(len(frame))*4 - 1
	buf[0] = byte(dibitCount)
	buf[1] = byte(dibitCount >> 8)
	n := 2 + copy(buf[2:], frame)

	r.tx.SetEnabled(false)
	r.tx.ClearFIFOs()
	r.tx.Restart()
	r.tx.Jmp(pio.JmpAlways, r.txOff)
	r.tx.SetEnabled(true)
	dreq := dmaPIO_TxDREQ(r.tx)
	err := r.dma.Push8((*byte)(unsafe.Pointer(&r.tx.TxReg().Reg)), buf[:n], dreq)
	if err != nil {
		return err
	}
	return nil
}
