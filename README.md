# pio
Provides clean API to interact with RP2040's on-board Programable Input/Output (PIO) block.
See page 306 of the [RP2040 datasheet](https://datasheets.raspberrypi.com/rp2040/rp2040-datasheet.pdf) for more information.


### piolib
This module contains the [piolib](./rp2040-pio/piolib) package which contains importable drivers for use with a PIO such as:

- SPI driver
- 8-pin send-only parallel bus
- WS2812 (Neopixel) driver
- A pulse-constrained square wave generator (Pulsar)


## Introduction to PIO
The PIO is a versatile hardware interface. It can support a variety of IO standards,
including:
- 8080 and 6800 parallel bus
- I2C
- 3-pin I2S
- SDIO
- SPI, DSPI, QSPI
- UART
- DPI or VGA (via resistor DAC)

PIO is programmable in the same sense as a processor and has a total of nine instructions: JMP, WAIT, IN, OUT, PUSH, PULL, MOV, IRQ, and SET. These are programmed in PIO assembly format describing a PIO program, where each command corresponds to one instruction in the output binary. Below is an example program in PIO assembly:

```pio
.program squarewave
again:
    set pins, 1 [1] ; Drive pin high and then delay for one cycle
    set pins, 0     ; Drive pin low
    jmp again       ; Set PC to label `again`
```
