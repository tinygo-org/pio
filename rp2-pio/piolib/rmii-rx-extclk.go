package piolib

import (
	"device/rp"
	"errors"
	"machine"
	"unsafe"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

type RMIIRxConfig struct {
	// Baud at which Rx will receive. Set to 100_000_000 for 100M operation.
	Baud uint32
	// RxBase is the first pin of the consecutive set [Rx0,Rx1,CRS_DV].
	// These pins must be consecutive and in that order.
	RxBase machine.Pin
	// IRQ is either 0 or 1 depending on IRQ line selected. Leave at 0 if unsure.
	// See rp2-pio/docs/irq-reference.md in this project for more info.
	IRQ uint8
	// IRQSource is the triggering source for state machine. Varies between 0..3 on RP2040 and extends to 0..7 on RP2350.
	IRQSourceIndex uint8
	// RefClk is the pin to REFCLK a.k.a RETCLK or EXTCLK. This is the external clock we synchronize with.
	RefClk machine.Pin
}

// RMIIRxExtClk is a PIO-based RMII receiver. It samples RX0, RX1 and CRS_DV at
// the RMII clock rate and delivers complete frames via an IRQ-driven callback.
type RMIIRxExtClk struct {
	sm       pio.StateMachine
	rxOff    uint8
	dma      dmaChannel
	rxbuf    []byte
	callback func(frame []byte)

	irqsource pio.IRQSource
	irq       uint8
	block     uint8
}

// Configure sets up the PIO state machine and DMA for RMII reception.
// The Baud field in cfg is ignored; PIO runs at the CPU clock frequency.
func (r *RMIIRxExtClk) Configure(PIO *pio.PIO, cfg RMIIRxConfig) error {
	if cfg.IRQSourceIndex > 3 {
		return errors.New("IRQSource index out of range (0-7)")
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

	rxBit := txPhase - 2 // Inter-sample delay.
	rxD0 := txPhase - 1  // SOF alignment delay.

	const (
		idxRX0 = iota
		idxRX1
		idxCRSDV

		polRising   = true
		labelSample = 4
		labelS2     = 6
	)

	/*
		Copyright (c) 2026 Patricio Whittingslow, with portions copyrighted as below
		Copyright (c) 2025 Rob Scott
		Copyright (c) 2021 Sandeep Mistry
	*/
	asm := pio.AssemblerV0{SidesetBits: 0}
	var rxprog = [...]uint16{
		asm.WaitPin(polRising, idxCRSDV).Encode(),                        // Wait for CRS_DV assertion.
		asm.Set(pio.SetDestX, 0).Encode(),                                // Init dibit counter.
		asm.WaitPin(polRising, idxRX1).Delay(uint8(rxD0)).Encode(),       // Wait for SOF + alignment delay.
		asm.WaitGPIO(false, uint8(cfg.RefClk)).Encode(),                  // Sync to RMII clock falling edge.
		labelSample:                                                      // Sample loop (2 dibits per iteration).
		asm.In(pio.InSrcPins, 2).Delay(uint8(rxBit)).Encode(),            // Sample dibit.
		asm.Jmp(pio.JmpXNZeroDec, labelS2).Encode(),                      // Timing spacer + counter decrement.
		labelS2:                                                          asm.In(pio.InSrcPins, 2).Delay(uint8(rxBit)).Encode(), // Sample dibit.
		asm.Jmp(pio.JmpPinInput, labelSample).Encode(),                   // Loop while CRS_DV high.
		asm.IRQSet(false, cfg.IRQSourceIndex).Encode(),                   // Signal end of frame.
	}
	rxSM, err := PIO.ClaimStateMachine()
	if err != nil {
		return err
	}
	rxoff, err := PIO.AddProgram(rxprog[:], -1)
	if err != nil {
		rxSM.Unclaim()
		return err
	}

	// Configure RX pins as inputs.
	rxPin := cfg.RxBase
	pinCfg := machine.PinConfig{Mode: PIO.PinMode()}
	for i := machine.Pin(0); i < 3; i++ {
		pin := rxPin + i
		pin.Configure(pinCfg)
	}
	// Create state machine configuration.
	rxcfg := asm.DefaultStateMachineConfig(rxoff, rxprog[:])
	rxcfg.SetInPins(rxPin, 2)         // IN pins: RX0, RX1 at rxPin.
	rxcfg.SetJmpPin(rxPin + idxCRSDV) // JMP pin: CRS_DV at rxPin+2.
	rxcfg.SetInShift(true, true, 8)   // Right shift, autopush at 8 bits (1 byte).
	rxcfg.SetFIFOJoin(pio.FifoJoinRx)
	rxcfg.SetClkDivIntFrac(1, 0) // Run at CPU clock.
	// Initialize SM at start of program.
	rxSM.Init(rxoff, rxcfg)
	// Set RX pins as inputs (pindirs = 0 for input).
	var rxPinMsk uint32 = 0b111 << rxPin
	rxSM.SetPindirsMasked(0, rxPinMsk)

	// Bypass input synchronizers for lower latency on RX pins and RefClk.
	syncBypassMsk := rxPinMsk | (1 << cfg.RefClk)
	PIO.SetInputSyncBypassMasked(syncBypassMsk, syncBypassMsk)
	r.dma.helperEnableDMA(true)
	r.sm = rxSM
	r.rxOff = rxoff

	err = PIO.SetInterrupt(cfg.IRQ, pio.IRQSFromSMIndex(cfg.IRQSourceIndex), r.irqhandler)
	if err != nil {
		PIO.ClearProgramSection(rxoff, uint8(len(rxprog)))
		rxSM.Unclaim()
		return err
	}
	r.block = 0xff
	r.irq = 0xff
	r.irqsource = 0xffffffff
	return nil
}

func (r *RMIIRxExtClk) irqhandler(pioblock, irqLine uint8, source pio.IRQSource) {
	// Halt ongoing rx async logic to wait for a future StartRx call.
	r.sm.SetEnabled(false)
	r.block = pioblock
	r.irq = irqLine
	r.irqsource = source
	if r.callback != nil {
		trans := r.dma.HW().TRANS_COUNT.Get()
		r.callback(r.rxbuf[:len(r.rxbuf)-int(trans)])
	}
	r.dma.abort()
}

// StopRx halts reception, aborts any in-flight DMA, and resets the state machine.
func (r *RMIIRxExtClk) StopRx() error {
	if !r.sm.IsEnabled() {
		return errors.New("already stopped")
	}
	r.sm.SetEnabled(false)
	if r.dma.busy() {
		r.dma.abort()
	}
	r.sm.ClearFIFOs()
	r.sm.Restart()
	r.sm.Jmp(pio.JmpAlways, r.rxOff)
	return nil
}

// StartRx begins frame reception. SetRxIRQHandler must be called first.
func (r *RMIIRxExtClk) StartRx() error {
	if len(r.rxbuf) == 0 {
		return errors.New("no rxbuf configured")
	} else if r.sm.IsEnabled() {
		return errors.New("already receiving")
	} else if !r.dma.IsClaimed() {
		return errors.New("need SetRxIRQHandler call before")
	}
	r.block = 0xff
	r.irq = 0xff
	r.irqsource = 0xffffffff
	hw := r.dma.HW()
	hw.CTRL_TRIG.ClearBits(rp.DMA_CH0_CTRL_TRIG_EN_Msk)
	// Configure and enable DMA.
	cc := r.dma.CurrentConfig()
	cc.setTREQ_SEL(dmaPIO_RxDREQ(r.sm))
	cc.setTransferDataSize(dmaTxSize8)
	cc.setChainTo(r.dma.idx)
	cc.setReadIncrement(false)
	cc.setWriteIncrement(true)
	cc.setEnable(true)
	// Configure DMA.
	hw.READ_ADDR.Set(uint32(uintptr(unsafe.Pointer(&r.sm.RxReg().Reg))) + 3) // +3 for byte access
	hw.WRITE_ADDR.Set(uint32(uintptr(unsafe.Pointer(&r.rxbuf[0]))))
	hw.TRANS_COUNT.Set(uint32(len(r.rxbuf)))
	r.dma.hw.CTRL_TRIG.Set(cc.CTRL)

	// Enable state machine in predicable way.
	r.sm.ClearFIFOs()
	r.sm.Restart()
	r.sm.Jmp(pio.JmpAlways, r.rxOff)
	r.sm.SetEnabled(true)
	return nil
}

// SetRxIRQHandler sets the receive buffer and callback invoked from the IRQ
// handler when a frame ends (CRS_DV deasserts). Stops any ongoing reception.
func (r *RMIIRxExtClk) SetRxIRQHandler(rxbuf []byte, callback func(buf []byte)) error {
	// Terminate ongoing transaction since we are tinkering with DMA target.
	r.StopRx()
	r.rxbuf = rxbuf
	r.callback = callback
	return nil
}

// ReceivedSinceStartRx returns true if a frame has been received since the last StartRx call.
func (r *RMIIRxExtClk) ReceivedSinceStartRx() bool {
	return r.block != 0xff || r.irq != 0xff || r.irqsource != 0xffff_ffff
}

// InRx returns true if the receiver state machine is currently enabled.
func (r *RMIIRxExtClk) InRx() bool {
	return r.sm.IsEnabled()
}
