package pio

// AssemblerV1 provides a fluent API for programming PIO
// within the Go language for PIO version 1 (RP2350).
// Most logic is shared with [AssemblerV0].
type AssemblerV1 struct {
	SidesetBits uint8
}

func (asm AssemblerV1) v0() AssemblerV0 {
	return AssemblerV0{
		SidesetBits: asm.SidesetBits,
	}
}

// Jmp instruction unchanged from [AssemblerV0.Jmp].
func (asm AssemblerV1) Jmp(addr uint8, cond JmpCond) instructionV0 { return asm.v0().Jmp(addr, cond) }

// WaitGPIO instruction unchanged from [AssemblerV0.WaitGPIO].
func (asm AssemblerV1) WaitGPIO(polarity bool, pin uint8) instructionV0 {
	return asm.v0().WaitGPIO(polarity, pin)
}

// WaitIRQ instruction unchanged from [AssemblerV0.WaitIRQ].
func (asm AssemblerV1) WaitIRQ(polarity bool, relative bool, irqindex uint8) instructionV0 {
	return asm.v0().WaitIRQ(polarity, relative, irqindex)
}

// WaitPin instruction unchanged from [AssemblerV0.WaitPin].
func (asm AssemblerV1) WaitPin(polarity bool, pin uint8) instructionV0 {
	return asm.v0().WaitPin(polarity, pin)
}

// WaitJmpPin waits on the pin indexed by the PINCTRL_JMP_PIN configuration, plus an Index in the range 0-3, all
// modulo 32. Other values of Index are reserved.
func (asm AssemblerV1) WaitJmpPin(polarity bool, pin uint8) instructionV0 {
	flag := boolAsU8(polarity) << 2
	return asm.v0().instrArgs(_INSTR_BITS_WAIT, 0b11|flag, pin)
}

// In instruction unchanged from [AssemblerV0.In].
func (asm AssemblerV1) In(src InSrc, value uint8) instructionV0 {
	return asm.v0().In(src, value)
}

// Out instruction unchanged from [AssemblerV0.Out].
func (asm AssemblerV1) Out(dest OutDest, value uint8) instructionV0 {
	return asm.v0().Out(dest, value)
}

// Push instruction unchanged from [AssemblerV0.Push].
func (asm AssemblerV1) Push(ifFull bool, block bool) instructionV0 {
	return asm.v0().Push(ifFull, block)
}

// Pull instruction unchanged from [AssemblerV0.Pull].
func (asm AssemblerV1) Pull(ifEmpty bool, block bool) instructionV0 {
	return asm.v0().Pull(ifEmpty, block)
}

// Mov in version 1 of PIO works identically to version 0 but adding following new functionality.
//   - Added Pindirs as destination for MOV:  This allows changing the direction of all OUT-mapped pins with a single instruction: MOV PINDIRS, NULL or MOV
//     PINDIRS, ~NULL
//   - Adds SM IRQ flags as a source for MOV x, STATUS. This allows branching (as well as blocking) on the assertion of SM IRQ flags.
//   - Adds the FJOIN_RX_GET FIFO mode. A new MOV encoding reads any of the four RX FIFO storage registers into OSR.
//   - New FJOIN_RX_PUT FIFO mode. A new MOV encoding writes the ISR into any of the four RX FIFO storage registers.
func (asm AssemblerV1) Mov(dest MovDest, src MovSrc) instructionV0 {
	return asm.v0().Mov(dest, src)
}

// MovInvert is [AssemblerV0.MovInvert] unchanged but with available [AssemblerV1.Mov] functionality.
func (asm AssemblerV1) MovInvert(dest MovDest, src MovSrc) instructionV0 {
	return asm.v0().MovInvert(dest, src)
}

// MovReverse is [AssemblerV0.MovReverse] unchanged but with available [AssemblerV1.Mov] functionality.
func (asm AssemblerV1) MovReverse(dest MovDest, src MovSrc) instructionV0 {
	return asm.v0().MovReverse(dest, src)
}

// MovOSRFromRx reads the selected RX FIFO entry into the OSR. The PIO state machine can read the FIFO entries in any order, indexed
// either by the Y register, or an immediate Index in the instruction. Requires the SHIFTCTRL_FJOIN_RX_GET configuration field
// to be set, otherwise its operation is undefined.
//   - If idxByImmediate (index by immediate) is set, the RX FIFO’s registers are indexed by the two least-significant bits of the Index
//     operand. Otherwise, they are indexed by the two least-significant bits of the Y register. When IdxI is clear, all non-zero
//     values of Index are reserved encodings, and their operation is undefined.
func (asm AssemblerV1) MovOSRFromRx(idxByImmediate bool, RxFifoIndex uint8) instructionV0 {
	instr := _INSTR_BITS_MOV | (0b1001 << 4) | (uint16(boolAsU8(idxByImmediate) << 3)) | uint16(RxFifoIndex)&0b111
	return asm.v0().instr(instr)
}

// MovISRToRx writes the ISR to a selected RX FIFO entry. The state machine can write the RX FIFO entries in any order, indexed either
// by the Y register, or an immediate Index in the instruction. Requires the SHIFTCTRL_FJOIN_RX_PUT configuration field to be
// set, otherwise its operation is undefined. The FIFO configuration can be specified for the program via the .fifo directive
// (see pioasm_fifo).
//   - If idxByImmediate (index by immediate) is set, the RX FIFO’s registers are indexed by the two least-significant bits of the Index
//     operand. Otherwise, they are indexed by the two least-significant bits of the Y register. When IdxI is clear, all non-zero
//     values of Index are reserved encodings, and their operation is undefined.
func (asm AssemblerV1) MovISRToRx(idxByImmediate bool, RxFifoIndex uint8) instructionV0 {
	instr := _INSTR_BITS_MOV | (0b1000 << 4) | (uint16(boolAsU8(idxByImmediate) << 3)) | uint16(RxFifoIndex)&0b111
	return asm.v0().instr(instr)
}

// Set instruction unchanged from [AssemblerV0.Set].
func (asm AssemblerV1) Set(dest SetDest, value uint8) instructionV0 {
	return asm.v0().Set(dest, value)
}

// IRQSet sets the IRQ flag selected by irqIndex.
func (asm AssemblerV1) IRQSet(irqIndex uint8, idxMode IRQIndexMode) instructionV0 {
	return asm.irq(false, false, irqIndex, idxMode)
}

// IRQClear clears the IRQ flag selected by irqIndex argument. See [AssemblerV1.IRQSet].
func (asm AssemblerV1) IRQClear(irqIndex uint8, idxMode IRQIndexMode) instructionV0 {
	return asm.irq(true, false, irqIndex, idxMode)
}

// IRQWait sets the IRQ flag selected by irqIndex and waits for it to be cleared before proceeding.
// If Wait is set, Delay cycles do not begin until after the wait period elapses.
func (asm AssemblerV1) IRQWait(irqIndex uint8, idxMode IRQIndexMode) instructionV0 {
	return asm.irq(false, true, irqIndex, idxMode)
}

func (asm AssemblerV1) irq(clear, wait bool, irqIndex uint8, idxMode IRQIndexMode) instructionV0 {
	instr := _INSTR_BITS_IRQ | uint16(boolAsU8(clear))<<6 | uint16(boolAsU8(wait))<<5 | uint16(idxMode&0b11)<<3 | uint16(irqIndex&0b111)
	return asm.v0().instr(instr)
}

// Nop instruction unchanged from [AssemblerV0.Nop].
func (asm AssemblerV1) Nop() instructionV0 { return asm.v0().Nop() }
