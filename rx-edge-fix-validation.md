# PIO design validation — RMII RX fix proposal

Web research validating the four fixes in `rx-edge-fix-proposal.md` against RP2040 PIO hardware
behavior and prior RMII-on-PIO work (notably rscott2049, whose design this code descends from).

**Bottom line:** all four fixes hold. #3 (RefClk-align) and #4 (reject frac) are strongly confirmed
by both the datasheet and rscott2049's own field notes. #1 is confirmed as a real fix but the
*mechanism* is metastability filtering, **not** a sample-phase shift — the proposal's own caveat was
right. Two new findings below (CRS async-assertion dibit slip, RefClk delay trick) strengthen the
case for #3 and #4-hardware.

## Fix 1 — stop bypassing input synchronizers: CONFIRMED (mechanism = metastability, not phase)

- RP2040 has a **2-flop synchronizer per GPIO input** that "protects PIO logic from metastabilities."
  Datasheet register note: `0 -> synchronized (default), 1 -> bypassed. If in doubt, leave this
  register as all zeroes.`
- Bypass cost/benefit measured: synchronizer adds **2 cycles** latency (~20 ns at the 100 MHz SM
  clock, ~7.5 ns at 266 MHz). With bypass, the SM samples the **raw async pad**, and per the
  datasheet "the behavior of the PIO state machine becomes undefined" under metastability — exactly
  the regime a slow RXD1 rising edge sits in.
- **Correction to the proposal's optimistic reading:** re-enabling sync delays the sampled input by
  2 cycles, but it delays the `WaitPin` alignment trigger by the **same** 2 cycles (both traverse the
  same synchronizer). So the *relative* phase between alignment and sampling does **not** move. The
  real win is metastability rejection on the marginal RXD1 edge, not a later sample point. Proposal
  caveat #2 was correct; keep the expectation framed as "filter metastability," not "shift phase."
- Verdict: valid, high-confidence, cheapest. Do first.

## Fix 2 — configurable + finer sample phase: CONFIRMED feasible

- Half-dibit (10 ns) granularity at 100 MHz / 2-cycle loop is real; going to 200 MHz `clkdiv=1` with a
  4-cycle loop (`In` + `Jmp` + 2 delay) gives 5 ns steps. No hardware obstacle — target already runs
  at 200 MHz.
- Independent confirmation the phase matters: rscott2049 notes the 50 MHz clock "is not phase locked
  to the PIO clocks on power up... leads to uncertainty in generation/sampling of the RMII bus."
  A tunable phase directly addresses that uncertainty.
- Verdict: valid as a fallback when RefClk wiring isn't available. Prefer #3 if it is.

## Fix 3 — align RX to RefClk: STRONGLY CONFIRMED (the structural fix)

- rscott2049's design generates the RMII clock **from the TX PIO** specifically because the free
  clock generator "is not phase locked to the PIO clocks," which "leads to uncertainty in
  generation/sampling." That is the same root cause `rx-edge-defect.md` measured — independent
  corroboration that free-running RX sampling is the defect, not a red herring.
- Feasibility / implementation note: the RX `In` base is already `[RX0,RX1,CRS_DV]`, so RefClk cannot
  reuse the `in`-pin mapping the way TX does. Use `wait gpio <n>` (absolute GPIO) for the RefClk edge
  instead of `wait pin`. At 200 MHz a dibit is 4 SM cycles, enough room for `wait`+`in`+`jmp` per
  dibit. The `wait` naturally paces the loop to RefClk, removing the free-run entirely.
- This also removes plesiochronous drift on 1518-byte frames (the ~12 ns / 60%-bit-cell case), which
  no other fix touches.
- Verdict: valid and structural. Highest-value fix if RefClk is wired.

## Fix 4 — reject fractional divider: CONFIRMED

- The fractional divider "runs the hardware for some cycles at the slower rate and some at the faster
  rate, so the average is between the two." It literally **stretches individual clock periods**, so
  the sample instant jitters dibit-to-dibit. Search sources: "For small integer divisors, a
  fractional divider introduces jitter."
- At the current 200 MHz target, `100e6/200e6 → whole=2, frac=0` (integer, no dither) — so the live
  build is safe, but nothing enforces it. A future 125/150 MHz target would silently reintroduce
  per-dibit sample jitter on top of the marginal RXD1 edge.
- Verdict: valid guard rail. Cheap, orthogonal, matches the existing TX check. Ship with #1.

## New findings (not in the proposal, worth folding in)

1. **CRS_DV async-assertion → undetectable dibit slip.** CRS is asserted asynchronously to RefClk, so
   its setup time can be violated at frame start; the first dibit pair shifts, and because "all
   preamble bit pairs look the same until the D nibble of the SFD," the slip is **invisible until
   SFD**. This is a second, distinct failure path from the SSO/slow-rise one in the defect doc, and
   it argues for aligning the SFD search to RefClk (fix #3) rather than to RXD1's own edge (current
   line 66). Consider it when designing the #3 state machine.

2. **RefClk trace-delay trick (supports #4-hardware).** rscott2049 reports adding a ~6" wire to REFCLK
   to insert **~960 ps** of delay "to balance the PHY's setup and hold margins." Direct precedent for
   the proposal's hardware step: RXD1 rise time and RefClk-vs-data skew are tunable at the board, and
   ~1 ns shifts matter at a 20 ns bit cell.

3. **DMA CRC sniffer (orthogonal, optional).** RP2040's DMA sniffer can offload CRC32, catching
   corrupted frames in hardware instead of software-after-callback. Doesn't fix the sampling defect
   but cheapens detection while iterating on the pass/fail metric.

## Sources

- [INPUT_SYNC_BYPASS register (rp2040_pac)](https://rtic.rs/dev/api/rp2040_pac/pio0/input_sync_bypass/struct.INPUT_SYNC_BYPASS_SPEC.html)
- [PIO wait latency, synchronizer cost (Hackaday)](https://hackaday.io/project/190347/log/217766-latency-rp2040-pio-wait)
- [PIO timing / setup-hold, metastability discussion (pico-feedback #280)](https://github.com/raspberrypi/pico-feedback/issues/280)
- [RP2040 PIO clock divider internals](https://rp2040.implrust.com/pio/internals/clock-divider.html)
- [PIO fractional divider jitter (Raspberry Pi forums)](https://forums.raspberrypi.com/viewtopic.php?t=375839)
- [rscott2049/pico-rmii-ethernet_nce — clock phase-lock + CRS async notes](https://github.com/rscott2049/pico-rmii-ethernet_nce/blob/main/README.md)
- [RMII sampling / CRS setup-time slip (Parallax forums)](https://forums.parallax.com/discussion/174351/rmii-ethernet-interface-driver-software)
- [RMII interface overview](https://etherealwake.com/2025/02/ethernet-rmii/)
