# Proposal: RefClk-aligned RMII RX

Fixes the `00`→`11` simultaneous-rise corruption in `rmii-rx-extclk.go` (see `rx-edge-defect.md`).
Root cause is that RX samples the async RXD pads on a free-running PIO clock at a phase set once
per frame off RXD1's own (marginal) rising edge. TX has zero errors because the PHY reclocks TX on
RefClk; RX has no equivalent reclock.

## Change

Land in cost order. Each step is independently measurable against the `00`→`11` bin (pass = drops
to ~0.0004 % floor).

### 1. Stop bypassing input synchronizers — 1 line

Remove/gate `PIO.SetInputSyncBypassMasked` at `rmii-rx-extclk.go:106`.

**Why:** bypass samples the raw async pad with no metastability filter, right where a slow RXD1
rising edge is still settling. Costs 2 cycles latency. Cheapest test — do first, but do not bank on
it (sync also delays the alignment trigger, so relative phase may not move; the real win here is
metastability filtering, not phase).

### 2. Make sample phase configurable + finer — small

Promote the hardcoded `Delay(1)` at `rmii-rx-extclk.go:66` to a config field. To get sub-half-dibit
granularity, run the SM at 200 MHz (`clkdiv=1`) with a 4-cycle read loop (`In` + `Jmp` + 2 delay
cycles) instead of 100 MHz / 2-cycle. That gives 5 ns phase steps vs the current 10 ns (= half a
dibit).

**Why:** current phase lands wherever the SFD edge fell in a 10 ns window. Sweeping against the
error-rate table lets us move the sample point off the data edge deliberately.

### 3. Align RX to RefClk, mirroring TX — structural fix

Add `RefClk machine.Pin` to `RMIIRxConfig`. `wait` on a defined RefClk edge per dibit, as
`RMIITxExtClk` does at frame start. Run the SM at CPU clock, integer `cpuFreq/50MHz` cycles per
dibit.

**Why:** removes the per-frame phase lottery *and* plesiochronous drift in one change. Only option
that also fixes 1518-byte frames, where drift alone eats ~60 % of a bit cell (12 ns). Makes the
sample point a design parameter, not an artifact of SFD arrival time.

### 4. Guard rail — regardless of the above

Reject `frac != 0` in `Configure` (`rmii-rx-extclk.go:45`), same as `RMIITxExtClk` rejects non-50MHz
multiples. A dithered PIO clock reintroduces exactly this class of edge-sampling bug. Of TX-legal
{100,150,200,250,300} MHz, only **100 / 200 / 300** give an integer RX divider at 100 Mbit.

## Recommendation

Ship **#1 + #4 now** (minutes, low risk). Measure. If `00`→`11` still above floor, do **#3** — it is
the only structural fix and the only one covering long frames. **#2** is a fallback if RefClk wiring
is unavailable on a given board.

## Validation

Reuse the `rx-edge-defect.md` method: capture fixed-size router→device ICMP echoes, reconstruct the
expected frame, bin dibits by `(prev, cur)`, report per-bin error rate. Need ~100 k frames
(flood-ping) for statistical power. Pass: `00`→`11` bin at floor, no bin > 0.01 %.
