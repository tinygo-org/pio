# pio

[![Build](https://github.com/tinygo-org/pio/actions/workflows/build.yml/badge.svg)](https://github.com/tinygo-org/pio/actions/workflows/build.yml)

Provides clean API to interact with RP2040's on-board Programable Input/Output (PIO) block.
See chapter 3 of the [RP2040 datasheet](https://datasheets.raspberrypi.com/rp2040/rp2040-datasheet.pdf#page=310) for more information.


### piolib
This module contains the [piolib](./rp2-pio/piolib) package which contains importable drivers for use with a PIO such as:

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

### How to install pioasm

To develop new code using PIO with this package, you must build and install the `pioasm` tool. It can be be built from the pico-sdk:

```shell
git clone git@github.com:raspberrypi/pico-sdk.git
cd pico-sdk/tools/pioasm
cmake .
make
sudo make install
```

### How to develop a PIO program

To develop a PIO program you first start out with the .pio file. Let's look at the Pulsar example first.

1. `pulsar.pio` specifies a binary PIO program that can be loaded to the PIO program memory.
2. `all_generate.go`: holds the code generation command on the line with `//go:generate pioasm -o go pulsar.pio     pulsar_pio.go` which by itself generates the raw binary code that can be loaded onto the PIO along with helper code to load it correctly inside `pulsar_pio.go`.
3. `pulsar_pio.go`: contains the generated code by the `pioasm` tool.
4. `pulsar.go`: contains the User facing code that allows using the PIO as intended by the author.

### Regenerating piolib

```shell
cd rp2-pio/piolib
go generate .
```

### Other notes

Keep in mind PIO programs are very finnicky, especially differentiating between SetOutPins and SetSetPins. The difference is subtle but it can be the difference between spending days debugging a silly conceptual mistake. Have fun!
