# Tufty parallel ST7789 example

This example drives the 320x240 ST7789 display on Pimoroni Tufty boards over
the 8-bit 8080-II parallel interface using RP2 PIO.

## Supported builds

TinyGo 0.43.0 or later includes targets for both boards.

Tufty 2040:

```sh
tinygo build -target=tufty2040 -o /tmp/tufty2040.uf2 .
```

Tufty 2350:

```sh
tinygo build -target=tufty2350 -o /tmp/tufty2350.uf2 .
```

For older TinyGo versions without the `tufty2350` target, use `pico-plus2`:

```sh
tinygo build -target=pico-plus2 -o /tmp/tufty2350.uf2 .
```

Tufty 2350 places `LCD_DB0..LCD_DB7` on `GPIO32..GPIO39`. The parallel PIO
helper selects the RP2350B `GPIOBASE=16` window and converts these physical
GPIO numbers to PIO-relative pin indices.

The board-specific setup also drives `POWER_EN` on `GPIO41` high before display
initialization. Without this, the backlight can turn on while the panel remains
blank.

## Board pin maps

Tufty 2040:

| Signal | GPIO |
| --- | --- |
| LCD_BACKLIGHT | GPIO2 |
| LCD_CS | GPIO10 |
| LCD_DC | GPIO11 |
| LCD_WR | GPIO12 |
| LCD_RD | GPIO13 |
| LCD_DB0..LCD_DB7 | GPIO14..GPIO21 |

Tufty 2350:

| Signal | GPIO |
| --- | --- |
| LCD_BACKLIGHT | GPIO26 |
| LCD_CS | GPIO27 |
| LCD_RS / LCD_DC | GPIO28 |
| LCD_WR | GPIO30 |
| LCD_RD | GPIO31 |
| LCD_DB0..LCD_DB7 | GPIO32..GPIO39 |
| POWER_EN | GPIO41 |

## Bus clock

The parallel PIO program has three instructions, so the PIO state machine runs
at `3 * busBaud`. Tufty 2040 uses the hardware-verified 15 MHz WR rate.
Tufty 2350 uses the same 15 MHz WR rate after hardware validation on the
RP2350B bus.
