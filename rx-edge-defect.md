# RMII RX: RXD1 loses simultaneous rising edges

Status: root-caused from field data, fix not yet attempted.
Affects `rp2-pio/piolib/rmii-rx-extclk.go` (`RMIIRx`). TX (`RMIITxExtClk`) is unaffected.

## Symptom

On a LAN8720 breakout driven by `RMIIRx` + `RMIITxExtClk` (RP2040 @ 200 MHz, 100M full duplex),
**1.14 % of received IPv4 frames arrive with a corrupted header** (3 234 of 283 546 measured over
6.3 days of continuous uptime). Corruption is bit-level, 1–14 bytes per frame, and lands on
whatever byte happens to be vulnerable — MAC addresses, IP addresses, length fields.

**Zero corruption on TX** (0 of 122 229 transmitted frames). That asymmetry is the first clue:
TX aligns every dibit to the PHY's RefClk, RX does not use RefClk at all.

Downstream this produces silently dropped frames (corrupted destination MAC/IP), stack demux
errors, and truncated/invalid-length frame errors.

## Root cause

**When RXD1 and RXD0 rise on the same RMII clock edge, RXD1 is sampled low ~1 % of the time.**

Every dibit transition, by observed error rate:

| prev → cur | RXD1 | RXD0 | observations | errors | rate |
|---|---|---|---|---|---|
| **`00` → `11`** | **rise** | **rise** | 366 954 | **3 663** | **0.998 %** |
| `01` → `11` | rise | — (high) | 366 954 | 250 | 0.068 % |
| `11` → `00` | fall | fall | 519 477 | 269 | 0.052 % |
| `01` → `10` | rise | fall | 244 636 | 54 | 0.022 % |
| `10` → `01` | fall | rise | 611 590 | 119 | 0.020 % |
| `00` → `10` | rise | — (low) | 856 226 | 96 | 0.011 % |
| `11` → `01` | fall | — | 366 954 | 28 | 0.008 % |
| `00` → `01` | — | rise | 1 100 862 | 25 | 0.002 % |
| `10` → `00` | fall | — | 519 744 | 6 | 0.001 % |
| no transition (5 rows) | — | — | 32 353 348 | 133 | 0.0004 % |

**3 663 of 3 982 errors (92 %) are the single `00` → `11` transition.** The failure is always the
same direction: the dibit reads back `01`, i.e. RXD1 lost and RXD0 won.

Summarised:

- RXD1 rising **with** RXD0: 0.998 %
- RXD1 rising **without** RXD0: 0.027 % — **37× lower**
- No edge at all: 0.0004 % — the floor

Secondary effects, both consistent with a marginal RXD1 rising edge:

- RXD1 rising while RXD0 sits high (`01`→`11`, 0.068 %) is 6× worse than RXD1 rising while RXD0
  sits low (`00`→`10`, 0.011 %).
- Simultaneous **falling** (`11`→`00`, 0.052 %) is 7–40× worse than single falls, but 20× better
  than simultaneous rising. Rise is the weak direction.
- At a *fixed* byte offset and dibit position, the error rate scales with how long RXD1 sat low
  first — 0.84 % at 3 dibits low, 1.42 % at 6, 2.24 % at 8. A slow rise that starts from a
  more-discharged line.

This is the signature of **simultaneous-switching sensitivity plus a slow rising edge on RXD1**,
sampled at a phase that is too early in the bit cell to tolerate it.

## What is ruled out

Worth recording so nobody re-derives it:

- **Not a fractional clock divider.** `Configure` computes the divider from `machine.CPUFrequency()`
  (`rmii-rx-extclk.go:45`). This TinyGo target runs RP2040 at **200 MHz**
  (`machine_rp2_2040.go:12`), so `ClkDivFromFrequency(100e6, 200e6)` → `whole=2, frac=0`. Integer,
  no dither. *(It would not be integer at 125 MHz or 150 MHz — see "Guard rails" below.)*
- **Not plesiochronous drift.** The PIO samples on its own 100 MHz clock while the PHY runs off its
  own crystal, but the measured frames are 102 bytes = 408 dibits = 8.16 µs. At ±100 ppm that is
  <1 ns of accumulated drift against a 20 ns bit cell. Cannot explain ~1 % error rates.
  *(It does become significant on 1518-byte frames: ~12 ns, 60 % of a bit cell.)*
- **Not depth into the frame.** Byte offset 26 is worse than offset 30 at identical run length —
  non-monotonic, so nothing is accumulating.
- **Not the wire.** IP header checksums are internally consistent with the *uncorrupted* packet
  (checksum deltas of −1, not 0x8000), so the PHY delivered good data and corruption happened at
  or after the pin. Corruption is also RX-only.

## Code-level suspects

In `rp2-pio/piolib/rmii-rx-extclk.go`:

1. **`PIO.SetInputSyncBypassMasked(rxPinMsk, rxPinMsk)` — line 106.** Bypasses the 2-flop input
   synchronizer on RXD0/RXD1/CRS_DV. The SM then samples the raw asynchronous pad with no
   metastability filtering, and effectively samples earlier in the bit cell. This is the single
   most suspicious line, and the cheapest thing to change.

2. **Sample phase is set once per frame and only to 10 ns granularity — line 66.**
   `asm.WaitPin(polRising, idxRX1).Delay(1)` aligns on the SFD, then the 2-instruction loop
   (`In` + `Jmp`, lines 68–69) free-runs at exactly one dibit per iteration. With the SM at
   100 MHz, one `Delay` unit is 10 ns = **half a bit cell**, so the phase is only tunable in
   half-bit steps and lands wherever the SFD edge fell within a 10 ns window. A frame that
   resyncs badly is sampled near the data edge for its entire length.

   Note this also means the alignment reference is **RXD1's rising edge** — the one signal that is
   demonstrably marginal. A late SFD edge poisons the phase for the whole frame.

3. **RX ignores RefClk entirely.** `RMIIRxConfig` has no `RefClk` field, while `RMIITxExtClk`
   requires one and waits on its falling edge for every dibit (`rmii-tx-extclk.go`, and its own
   doc comment contrasts itself with the free-running variant). TX has zero errors; RX has 1.14 %.

## Fix plan

Ordered by cost. Each step has the same pass/fail metric (below), so they can be evaluated
independently.

### 1. Stop bypassing the input synchronizers (one line, minutes)

Drop or make optional the `SetInputSyncBypassMasked` call at line 106. Costs 2 cycles of input
latency — which shifts the effective sample point later in the bit cell, which is the direction we
want anyway. If this alone fixes it, done.

### 2. Sweep the sample phase (small, mechanical)

Make the `Delay(1)` on line 66 a config field rather than a hardcoded constant, and sweep it. The
existing comment ("Delay modified from rscott version, yields better results") says this was already
hand-tuned once by eye; tune it against the error-rate table instead.

To get finer than half-a-bit granularity, run the SM at 200 MHz (`clkdiv = 1`) with a 4-cycle loop
instead of 100 MHz with a 2-cycle loop — `In` + `Jmp` + 2 delay cycles. That gives 5 ns phase
steps. This requires `CPUFrequency() == 200 MHz`, which is already the target's value.

### 3. Align RX to RefClk, as TX does (the structural fix)

Add `RefClk machine.Pin` to `RMIIRxConfig` and `wait` on a defined RefClk edge for each dibit,
mirroring `RMIITxExtClk`. This removes the per-frame phase lottery and the plesiochronous drift on
long frames in one change, and makes the sample point a deliberate design parameter rather than an
artifact of when the SFD arrived.

This is the only option that also fixes 1518-byte frames, where drift alone eats 60 % of a bit cell.

### 4. Hardware, if the above does not close it

Sensitivity to *simultaneous* switching specifically points at shared impedance rather than at one
trace: ground return path and supply decoupling on the breakout, then series termination and
capacitive load on RXD1 relative to RXD0. Scope RXD1's rise time at the RP2040 pin, triggering on a
`00`→`11` dibit, and compare against RXD0.

### Guard rails worth adding regardless

`Configure` silently accepts a fractional divider. `RMIITxExtClk.Configure` already rejects a CPU
frequency that is not a multiple of 50 MHz; RX should likewise reject `frac != 0`, since a dithered
PIO clock would reintroduce exactly this class of bug. Of the TX-legal frequencies
{100, 150, 200, 250, 300} MHz, only **100, 200 and 300** give an integer RX divider at 100 Mbit.

## How to measure (pass/fail)

The metric is the transition table above, rebuilt from captured traffic. Method used to produce it:

1. Capture RX frames with the pcap printer for a few hours on a live LAN.
2. Select router→device ICMP echo requests of a fixed size (102 B here). Every byte of these is
   known a priori except the IP id, the two checksums, and the ping timestamp, so the *expected*
   frame can be reconstructed exactly and compared byte-for-byte against what was received.
3. Convert expected and received to RMII dibits (LSB-first: `d[n] = (byte >> 2n) & 3`) and bin every
   dibit by `(previous expected dibit, current expected dibit)`.
4. Report error rate per bin.

**Pass condition:** the `00`→`11` bin drops to the no-transition floor (~0.0004 %) and no bin
exceeds ~0.01 %. Partial credit is measurable — a fix that halves it will show as a halved rate,
so steps 1–3 can be evaluated one at a time.

Sample size matters: at 0.001 % you need ~10⁶ transitions of a given type to see anything, which is
roughly 100 k frames. An hour of ping traffic at 1 Hz is not enough; flood-ping or a traffic
generator is.
