# ST7789 8080-parallel example for Tufty 2040

This example drives the Tufty 2040's ST7789 display over its 8-bit 8080-style
parallel interface using one RP2040 PIO state machine. The PIO program holds
`WR` low while it presents `DB0..DB7`, pulses `WR` high for one PIO cycle to
latch the byte, and returns `WR` low before the next byte.

The display's fixed board wiring is:

| Signal | RP2040 pin |
| --- | --- |
| `LCD_CS` | GPIO10 |
| `LCD_DC` | GPIO11 |
| `LCD_WR` | GPIO12 |
| `LCD_RD` | GPIO13 |
| `LCD_DB0..LCD_DB7` | GPIO14..GPIO21 |
| `LCD_BACKLIGHT` | GPIO2 |

`LCD_RD` is held high because this is a write-only example. The display uses
RGB565 pixels, sent high byte first. The program first runs the ST7789 reset
and setup sequence, then cycles bouncing-rectangle, Mandelbrot, and plasma
demos through all four rotations.

## Build and flash

With TinyGo and a Tufty 2040 connected by USB, build and flash the example:

```shell
tinygo flash -target tufty2040 ./rp2-pio/examples/parallel/tufty
```

The RP2040 PIO/DMA build can also be checked without a board:

```shell
tinygo build -target pico-w -size short -o build/tufty.uf2 ./rp2-pio/examples/parallel/tufty
```

## Hardware validation checklist

1. Confirm that the backlight stays off during initialization and turns on
   after `DISPON`.
2. Confirm that the three animated demos display without tearing or corrupted
   pixels.
3. Confirm that each 90-degree rotation uses the entire display rather than
   clipping or transposing the image.

The `parallel8_write_cycle` unit test in `rp2-pio/pio_test.go` guards the PIO
instruction encoding for the low/high/low `WR` waveform. It is a software
check only; it does not replace hardware validation.
