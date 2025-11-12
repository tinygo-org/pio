package pio

import (
	"testing"
)

func TestAssemblerV0_spi3w(t *testing.T) {
	assm := AssemblerV0{
		SidesetBits: 1,
	}
	const (
		wloopOff = 0
		rloopOff = 5
		endOff   = 7
	)

	var program = []uint16{
		//     .wrap_target
		// write out x-1 bits.
		wloopOff:// Write/Output loop.
		assm.Out(OutDestPins, 1).Side(0).Encode(), //  0: out    pins, 1         side 0
		assm.Jmp(wloopOff, JmpXNZeroDec).Side(1).Encode(), //  1: jmp    x--, 0          side 1
		assm.Jmp(endOff, JmpYZero).Side(0).Encode(),       //  2: jmp    !y, 7           side 0
		assm.Set(SetDestPindirs, 0).Side(0).Encode(),      //  3: set    pindirs, 0      side 0
		assm.Nop().Side(0).Encode(),                       //  4: nop                    side 0
		// read in y-1 bits.
		rloopOff:// Read/input loop
		assm.In(InSrcPins, 1).Side(1).Encode(), // 5: in     pins, 1         side 1
		assm.Jmp(rloopOff, JmpYNZeroDec).Side(0).Encode(), //  6: jmp    y--, 5          side 0
		// Wait for SPI packet on IRQ.
		endOff:// Wait on input pin.
		assm.WaitPin(true, 0).Side(0).Encode(), //  7: wait   1 pin, 0        side 0
		assm.IRQSet(false, 0).Side(0).Encode(), //  8: irq    nowait 0        side 0
	}
	var expectedProgram = []uint16{
		//     .wrap_target
		0x6001, //  0: out    pins, 1         side 0
		0x1040, //  1: jmp    x--, 0          side 1
		0x0067, //  2: jmp    !y, 7           side 0
		0xe080, //  3: set    pindirs, 0      side 0
		0xa042, //  4: nop                    side 0
		0x5001, //  5: in     pins, 1         side 1
		0x0085, //  6: jmp    y--, 5          side 0
		0x20a0, //  7: wait   1 pin, 0        side 0
		0xc000, //  8: irq    nowait 0        side 0
		//     .wrap
	}

	for i := range program {
		if program[i] != expectedProgram[i] {
			t.Errorf("instr %d mismatch got!=expected: %#x != %#x", i, program[i], expectedProgram[i])
		}
	}
}
