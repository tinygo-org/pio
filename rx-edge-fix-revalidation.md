# Re-validation of the RMII RX fix proposal

Second-pass review of `rx-edge-defect.md`, `rx-edge-fix-proposal.md` and `rx-edge-fix-validation.md`,
this time checked against the actual code and primary sources. The root-cause analysis (simultaneous
switching + marginal RXD1 rising edge, sampled at an uncontrolled phase) **stands**. Several
supporting claims do not. This file restates what is actually true so it can be read standalone.

**Bottom line:** ship **#3 (RefClk-aligned RX)** as the fix and **#4 (reject fractional divider)**
as the guard rail. **#1 (re-enable input synchronizers)** is hygiene — ship it, but expect **no
measurable change** in the error rate from it. **#2** remains the fallback when RefClk is not wired.

## How TX actually works — and what the asymmetry really proves

`rx-edge-defect.md` claimed TX waits on RefClk's falling edge *for every dibit*. The code says
otherwise: `RMIITxExtClk` waits on RefClk **once per frame** (`rmii-tx-extclk.go:116-117`), then
free-runs at CPU clock with fixed instruction delays for the rest of the frame — same free-running
structure as RX.

This changes the interpretation of the TX/RX asymmetry:

- **TX and RX have identical plesiochronous drift exposure.** Both align once and free-run. TX shows
  0 errors in 122 229 frames, so the "~12 ns drift on 1518-byte frames" concern is overstated —
  IEEE 802.3 mandates ±50 ppm per side (worst case ~12 ns over 1518 B), but typical crystals sit at
  ±20–30 ppm, and zero TX corruption is direct evidence the real-world drift fits inside the margin.
  Fix #3 still removes drift as a class; it is just not the load-bearing argument.
- **The real asymmetry is the alignment reference.** TX aligns to a clean RefClk edge with a tuned
  delay (`txD0`). RX aligns to RXD1's own rising edge — the one signal shown to be marginal — at
  whatever phase the SFD detection happened to quantize to. That is the defect, and it is exactly
  what #3 fixes.

## Fix 1 (re-enable input synchronizers): hygiene, not a fix

The original validation correctly showed that re-enabling sync cannot shift the sample phase (the
`wait` trigger and the `in` samples are delayed by the same 2 cycles), but then still called it a
"confirmed real fix" via metastability filtering. That does not hold:

- The first synchronizer flop samples the **same raw pad at the same instant** the bypassed SM
  would. A slow RXD1 rise sitting below threshold reads a clean, deterministic, **wrong** 0 either
  way. The synchronizer resolves metastable *levels*; it does not correct a wrong-phase sample.
- The failure signature — ~1 % rate, always the same direction (RXD1 loses), scaling with
  simultaneous switching — is a deterministic timing failure. True metastability through a 2-flop
  chain has an MTBF measured in years, not 1-in-100. The datasheet's promise is only that the SM
  "will see a clean high or low level" (§3.5.6.3) — clean, not correct.
- The earlier quote "the behavior of the PIO state machine becomes undefined" is **not in the
  datasheet**. The datasheet says the synchronizers "protect PIO from metastabilities" and the
  register note says "If in doubt, leave this register as all zeroes." The "unspecified" language
  comes from community discussion of the missing setup/hold specs (pico-feedback #280), which also
  establishes that the synchronizers and the bypass setup/hold window are relative to **clk_sys**
  (200 MHz here → ~10 ns latency), not the divided SM clock.

Verdict: re-enable sync because the datasheet says to and it costs nothing, but do not spend a
measurement cycle expecting it to move the `00`→`11` bin. If it is measured alone and does nothing,
that is the expected outcome, not a failed experiment.

## Fix 3 (RefClk-aligned RX): still the fix, with design corrections

- rscott2049's README confirms the principle verbatim: "the 50 MHz clock generator output is not
  phase locked to the PIO clocks on power up." Note his remedy was the opposite topology — the
  RP2040 *generates* RefClk from the TX PIO — so it supports the phase-lock principle but is not a
  direct precedent for waiting on a PHY-sourced RefClk.
- `wait pin` **can** reach RefClk: its index is relative to `IN_BASE` mod 32, independent of the
  `in pins` count, so `wait pin (RefClk − RxBase) mod 32` works. `wait gpio` remains the simpler
  choice; the earlier claim that the in-pin mapping blocks it was wrong.
- Timing budget is tighter than stated. Edge detection needs *two* waits per dibit
  (`wait 0` + `wait 1` + `in` + `jmp`) = exactly 4 cycles at 200 MHz with **zero slack**; a missed
  edge slips a full dibit. The synchronizers add 2 clk_sys cycles (~10 ns = half a dibit cell) to
  when the RefClk edge is *seen*, so the sample lands roughly 3/4 into the cell. Feasible, but the
  edge choice and latency must be designed deliberately, not assumed.

## Fix 4 (reject fractional divider): unchanged

Valid as stated. TinyGo's rp2040 target runs at 200 MHz (`machine_rp2_2040.go:12`,
`cpuFreq = 200 * MHz`), so the live build gets an integer divider; the check prevents a future
125/150 MHz target from silently reintroducing per-dibit sample jitter. Integer-divider set at
100 Mbit remains {100, 200, 300} MHz.

## Dropped and corrected findings

- **CRS_DV dibit-slip ("new finding 1") does not apply to this code.** That failure mode belongs to
  designs that start shifting dibits at CRS_DV assertion (the Parallax/Propeller driver, where the
  observation originates). This program waits for CRS_DV and *then* for RXD1 high
  (`rmii-rx-extclk.go:65-66`); preamble dibits are all `01`, so RXD1 stays low until the final SFD
  `11` dibit and byte alignment is independent of when CRS_DV asserts. The Parallax thread quotes
  ("CRS is asserted asynchronously", "all preamble bit pairs look the same until the D nibble of the
  SFD") are real but describe the other design.
- **The ~960 ps REFCLK wire-delay trick ("new finding 2") is unsourced.** It is not in rscott2049's
  README and no source for the figure was found. Board-level skew tuning is still a legitimate
  hardware step; the specific precedent is withdrawn.
- **Defect-doc bookkeeping errors** (do not affect the root cause, do affect the pass/fail table):
  the error column sums to 4 643, not 3 982, making the `00`→`11` share **78.9 %**, not 92 %; "no
  transition (5 rows)" is impossible (only 4 self-transitions exist) and three edge bins
  (`10`→`11`, `11`→`10`, `01`→`00`) are missing from the table entirely — if they had zero
  observations in the fixed ICMP payload, the table should say so. Rebuild the table before using
  it as the pass/fail baseline.
- **"More-discharged line" explanation is physically dubious.** A CMOS push-pull line settles to
  rail in nanoseconds; sitting low for 60 ns vs 160 ns discharges nothing further. The run-length
  correlation may be real but likely reflects a confound (byte offset, neighboring-pin switching
  activity). Scope RXD1 before believing the mechanism.

## Revised recommendation

1. Ship **#4** now (guard rail, zero risk).
2. Ship **#1** now as hygiene, with the stated expectation of *no* rate change.
3. Implement **#3** as the fix, with the wait-loop timing designed around the 4-cycle budget and
   clk_sys synchronizer latency above. Measure against the rebuilt transition table.
4. Keep **#2** as the fallback for boards without RefClk wired to the RP2040.

Pass/fail metric unchanged: `00`→`11` bin drops to the ~0.0004 % no-transition floor, no bin above
~0.01 % — but computed from a corrected table (all 16 bins accounted for, totals that sum).

## Sources

- [RP2040 datasheet §3.5.6.3 input synchronisers, INPUT_SYNC_BYPASS](https://datasheets.raspberrypi.com/rp2040/rp2040-datasheet.pdf)
- [pico-feedback #280 — PIO setup/hold unspecified, relative to clk_sys](https://github.com/raspberrypi/pico-feedback/issues/280)
- [rscott2049/pico-rmii-ethernet_nce README — phase-lock quote (verified); wire-delay claim absent](https://github.com/rscott2049/pico-rmii-ethernet_nce/blob/main/README.md)
- [Parallax RMII thread — CRS_DV async assertion, preamble/SFD quotes (other design)](https://forums.parallax.com/discussion/174351/rmii-ethernet-interface-driver-software)
- [Raspberry Pi forums — input synchronisers behaviour](https://forums.raspberrypi.com/viewtopic.php?t=317857)
- Code: `rp2-pio/piolib/rmii-tx-extclk.go:116-117` (single per-frame RefClk wait),
  `rp2-pio/piolib/rmii-rx-extclk.go:65-66` (CRS_DV → RXD1 alignment), TinyGo
  `machine_rp2_2040.go:12` (200 MHz).
