# PIO Interrupt Registers Reference

This document describes PIO interrupt handling on RP2040/RP2350.

---

## Quick Reference

### Register Offsets

| Register | RP2040 | RP2350 | Description |
|----------|--------|--------|-------------|
| IRQ | 0x030 | 0x030 | State machine IRQ flags (write-1-to-clear) |
| IRQ_FORCE | 0x034 | 0x034 | Force IRQ flags (for testing) |
| INTR | 0x128 | 0x16C | Raw interrupt status (before masking) |
| IRQ0_INTE | 0x12C | 0x170 | Interrupt enable for IRQ line 0 |
| IRQ0_INTF | 0x130 | 0x174 | Interrupt force for IRQ line 0 |
| IRQ0_INTS | 0x134 | 0x178 | Interrupt status for IRQ line 0 (masked) |
| IRQ1_INTE | 0x138 | 0x17C | Interrupt enable for IRQ line 1 |
| IRQ1_INTF | 0x13C | 0x180 | Interrupt force for IRQ line 1 |
| IRQ1_INTS | 0x140 | 0x184 | Interrupt status for IRQ line 1 (masked) |

### IRQSource Constants

| Constant | Value | Bit | Description |
|----------|-------|-----|-------------|
| IRQSRxFIFONotEmpty0 | 0x001 | 0 | SM0 RX FIFO not empty |
| IRQSRxFIFONotEmpty1 | 0x002 | 1 | SM1 RX FIFO not empty |
| IRQSRxFIFONotEmpty2 | 0x004 | 2 | SM2 RX FIFO not empty |
| IRQSRxFIFONotEmpty3 | 0x008 | 3 | SM3 RX FIFO not empty |
| IRQSTxFIFOHasSpace0 | 0x010 | 4 | SM0 TX FIFO has space |
| IRQSTxFIFOHasSpace1 | 0x020 | 5 | SM1 TX FIFO has space |
| IRQSTxFIFOHasSpace2 | 0x040 | 6 | SM2 TX FIFO has space |
| IRQSTxFIFOHasSpace3 | 0x080 | 7 | SM3 TX FIFO has space |
| IRQS0 | 0x100 | 8 | PIO IRQ flag 0 |
| IRQS1 | 0x200 | 9 | PIO IRQ flag 1 |
| IRQS2 | 0x400 | 10 | PIO IRQ flag 2 |
| IRQS3 | 0x800 | 11 | PIO IRQ flag 3 |
| IRQS4 | 0x1000 | 12 | PIO IRQ flag 4 (RP2350 only) |
| IRQS5 | 0x2000 | 13 | PIO IRQ flag 5 (RP2350 only) |
| IRQS6 | 0x4000 | 14 | PIO IRQ flag 6 (RP2350 only) |
| IRQS7 | 0x8000 | 15 | PIO IRQ flag 7 (RP2350 only) |


### NVIC IRQ Numbers

| IRQ Name | RP2040 | RP2350 | Description |
|----------|--------|--------|-------------|
| IRQ_PIO0_IRQ_0 | 7 | 15 | PIO0 interrupt line 0 |
| IRQ_PIO0_IRQ_1 | 8 | 16 | PIO0 interrupt line 1 |
| IRQ_PIO1_IRQ_0 | 9 | 17 | PIO1 interrupt line 0 |
| IRQ_PIO1_IRQ_1 | 10 | 18 | PIO1 interrupt line 1 |
| IRQ_PIO2_IRQ_0 | - | 19 | PIO2 interrupt line 0 |
| IRQ_PIO2_IRQ_1 | - | 20 | PIO2 interrupt line 1 |

---

## Overview

PIO interrupts involve three levels:
1. **PIO-level IRQ flags** (8 flags per PIO block, set by state machine instructions)
2. **Interrupt routing** (selecting which sources trigger which CPU interrupt line)
3. **NVIC** (ARM Cortex-M interrupt controller)

```
┌────────────────────────────────────────────────────────────────────────────┐
│                              PIO Block                                     │
│  ┌──────────────┐                                                          │
│  │ State Machine│──IRQSet(0)──►┌─────────────────┐                         │
│  │              │              │   IRQ Register  │                         │
│  │              │              │   (0x30)        │                         │
│  └──────────────┘              │   bits 0-7     │                          │
│                                └────────┬────────┘                         │
│                                         │                                  │
│         ┌───────────────────────────────┼──────────────────────────────┐   │
│         │                               ▼                              │   │
│         │  ┌─────────────┐    ┌─────────────────┐    ┌─────────────┐   │   │
│         │  │ FIFO Status │    │  IRQ flags 0-7  │    │ FIFO Status │   │   │
│         │  │  (RX/TX)    │    │  (from 0x30)    │    │  (RX/TX)    │   │   │
│         │  │  bits 0-7   │    │   bits 8-15     │    │  bits 0-7   │   │   │
│         │  └──────┬──────┘    └────────┬────────┘    └──────┬──────┘   │   │
│         │         │                    │                    │          │   │
│         │         ▼                    ▼                    ▼          │   │
│         │  ┌────────────────────────────────────────────────────────┐  │   │
│         │  │              IRQ_INT[0].E  (INTE)                      │  │   │
│         │  │              RP2040: 0x12C | RP2350: 0x170             │  │   │
│         │  │              Interrupt Enable Mask                     │  │   │
│         │  │              (RP2040: 12 bits | RP2350: 16 bits)       │  │   │
│         │  └────────────────────────────┬───────────────────────────┘  │   │
│         │                               │ AND                          │   │
│         │                               ▼                              │   │
│         │  ┌────────────────────────────────────────────────────────┐  │   │
│         │  │              IRQ_INT[0].S  (INTS)                      │  │   │
│         │  │              RP2040: 0x134 | RP2350: 0x178             │  │   │
│         │  │              Interrupt Status (masked)                 │  │   │
│         │  └────────────────────────────┬───────────────────────────┘  │   │
│         │                               │ any bit set?                 │   │
│         │                               ▼                              │   │
│         │                      ┌─────────────────┐                     │   │
│         │                      │  IRQ_PIO0_IRQ_0 │──────────────────────────►NVIC
│         │                      │  (to CPU)       │                     │   │
│         │                      └─────────────────┘                     │   │
│         │                                                              │   │
│         │  (Same structure for IRQ_INT[1] → IRQ_PIO0_IRQ_1)            │   │
│         └──────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Key Concepts

### Bit Mapping: IRQ Register ↔ INTE/INTS

The IRQ register (8 flags) maps to bits 8-15 of INTR/INTE/INTS:

```
IRQ register bit N  →  INTR/INTE/INTS bit (8 + N)
```

| IRQ Register | INTE/INTS |
|--------------|-----------|
| Bit 0 | Bit 8 |
| Bit 1 | Bit 9 |
| Bit 2 | Bit 10 |
| Bit 3 | Bit 11 |
| Bit 4 | Bit 12 |
| Bit 5 | Bit 13 |
| Bit 6 | Bit 14 |
| Bit 7 | Bit 15 |

To extract IRQ flags from INTS:
```go
irqFlags := uint8((stat >> 8) & 0xFF)
```

### Platform Differences: RP2040 vs RP2350

| Feature | RP2040 | RP2350 |
|---------|--------|--------|
| IRQ flags for CPU interrupts | 0-3 only | 0-7 (all) |
| IRQ flags for SM sync | 0-7 | 0-7 |
| INTE/INTS valid bits | 0x0FFF (12 bits) | 0xFFFF (16 bits) |
| Number of PIO blocks | 2 (PIO0, PIO1) | 3 (PIO0, PIO1, PIO2) |
| INTR offset | 0x128 | 0x16C |
| IRQn_INTE/F/S offsets | 0x12C-0x140 | 0x170-0x184 |

**RP2040 Limitation**: IRQ flags 4-7 exist in the IRQ register and can be used for inter-state-machine synchronization (`IRQ WAIT`, `IRQ CLEAR`), but they **cannot trigger CPU interrupts** because they're not connected to INTR/INTE/INTS.

### REL Addressing

The `REL` modifier in IRQ instructions adds the state machine ID (0-3) to the lower 2 bits:

```
IRQ SET <n> REL  →  sets flag (n & 4) | ((n + SM_ID) & 3)
```

This keeps IRQs 0-3 in range 0-3 and IRQs 4-7 in range 4-7, while allowing each SM to use its own flag.

Example: `IRQ SET 5 REL` on SM2 → `(5 & 4) | ((5 + 2) & 3)` = `4 | 3` = **7**

---

## Usage Guide

### Setting Up Interrupts

```go
// 1. Register the interrupt handler with TinyGo runtime
interrupt.New(rp.IRQ_PIO0_IRQ_0, handleInterrupt).Enable()

// 2. Enable the interrupt in NVIC
rp.PPB.NVIC_ICPR.Set(1 << rp.IRQ_PIO0_IRQ_0)  // Clear pending
rp.PPB.NVIC_ISER.Set(1 << rp.IRQ_PIO0_IRQ_0)  // Enable

// 3. Enable the interrupt source in PIO
hw := pio.HW()
hw.IRQ_INT[0].E.SetBits(uint32(pio.IRQS0))  // Enable IRQS0 on line 0
```

Each PIO has **two** independent interrupt lines (IRQ0 and IRQ1). You can route different sources to different lines.

### Handling Interrupts

```go
func handleInterrupt(intr interrupt.Interrupt) {
    hw := pio.HW()
    stat := hw.IRQ_INT[0].S.Get()  // Read IRQ0_INTS

    // Check which sources fired
    if stat&uint32(pio.IRQS0) != 0 {
        // IRQ flag 0 caused this interrupt
    }
    if stat&uint32(pio.IRQSRxFIFONotEmpty0) != 0 {
        // SM0 RX FIFO has data
    }

    // Clear PIO IRQ flags (bits 8-15 of stat map to bits 0-7 of IRQ reg)
    irqFlags := uint8((stat >> 8) & 0xFF)
    if irqFlags != 0 {
        hw.IRQ.Set(uint32(irqFlags))  // Write-1-to-clear
    }

    // Call user callback
    callback(...)
}
```

**Clearing behavior:**
- **FIFO interrupts** (bits 0-7): Clear automatically when condition resolves (RX emptied, TX filled)
- **PIO IRQ flags** (bits 8-15): Must be explicitly cleared by writing to the IRQ register

### Debugging

| Symptom | Possible Cause | Solution |
|---------|----------------|----------|
| Handler never called | IRQ flag never set | Check `pio.GetIRQ()` - is the flag being set? Verify PIO program reaches `IRQ SET` instruction |
| Handler never called | Source not enabled | Check `hw.IRQ_INT[0].E.Get()` - should show your IRQSource bit |
| Handler never called | NVIC not enabled | Check `rp.PPB.NVIC_ISER.Get()` - should have PIO IRQ bit set |
| Handler never called | Wrong interrupt line | Verify you're enabling IRQ_INT[0] but registering for IRQ_PIO0_IRQ_0 (not _1) |
| Infinite interrupt loop | Forgot to clear flag | Add `hw.IRQ.Set(uint32(irqFlags))` in handler |
| Wrong IRQSource value | Using bit position not mask | Use `IRQS0` (0x100), not `8` |
| RP2040: flags 4-7 don't trigger | Platform limitation | Flags 4-7 can't trigger CPU interrupts on RP2040 (use 0-3) |

**Debug prints:**
```go
fmt.Printf("IRQ reg = 0x%02x\n", pio.GetIRQ())
fmt.Printf("INTE[0] = 0x%04x\n", hw.IRQ_INT[0].E.Get())
fmt.Printf("INTS[0] = 0x%04x\n", hw.IRQ_INT[0].S.Get())
fmt.Printf("NVIC_ISER = 0x%08x\n", rp.PPB.NVIC_ISER.Get())
```

---

## Register Reference

### IRQ Register (0x030)

Contains 8 PIO IRQ flags set/cleared by state machine instructions.

| Bits | Access | Description |
|------|--------|-------------|
| 31:8 | - | Reserved |
| 7:0 | R/W1C | IRQ flags 7-0 (write 1 to clear) |

**Set by:** `IRQ SET <n>` instruction
**Cleared by:** Writing 1 to bit, `IRQ CLEAR <n>`, or `IRQ WAIT <n>` (after wait completes)

### IRQ_FORCE Register (0x034)

Force IRQ flags for testing. Writing here sets actual flags in the IRQ register, visible to state machines.

| Bits | Access | Description |
|------|--------|-------------|
| 31:8 | - | Reserved |
| 7:0 | WO | Writing 1 to bit n sets IRQ flag n |

**Note:** Unlike INTF (which only forces CPU interrupt routing), IRQ_FORCE writes to the actual IRQ register, so state machines can see these flags via `IRQ WAIT` or `IRQ CLEAR`.

### INTR, INTE, INTF, INTS Registers

These four registers share the same bit layout (see offsets in Quick Reference):

| Bits | Name | Description |
|------|------|-------------|
| 15:8 | IRQS7-0 | PIO IRQ flags 7-0 (RP2040: only 11:8 valid) |
| 7:4 | TX FIFO | TX FIFO not full for SM3-0 |
| 3:0 | RX FIFO | RX FIFO not empty for SM3-0 |

**Register functions:**
- **INTR** (Read-only): Raw interrupt status before masking
- **INTE** (Read/Write): Selects which sources are routed to this CPU interrupt line
- **INTF** (Read/Write): Force interrupt sources for testing (CPU-side only, not visible to SMs)
- **INTS** (Read-only): Masked status. Formula: `INTS = (INTR | INTF) & INTE`

---

## References

- RP2040 Datasheet, Section 3.7 (PIO)
- RP2350 Datasheet, Section 3.7 (PIO)
- pico-sdk: `hardware/pio.h`, `hardware/regs/pio.h`
