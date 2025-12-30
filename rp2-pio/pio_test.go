package pio

import (
	"testing"
)

func TestAssemblerV0(t *testing.T) {
	asm0 := AssemblerV0{SidesetBits: 0}
	asm1 := AssemblerV0{SidesetBits: 1}
	asm2 := AssemblerV0{SidesetBits: 2}
	var tests = []struct {
		name       string
		program    []uint16
		expectprog []uint16
	}{
		{
			name: "pulsar",
			program: []uint16{
				0: asm0.Set(SetDestPindirs, 1).Encode(),
				1: asm0.Pull(false, true).Encode(),
				2: asm0.Mov(MovDestX, MovSrcOSR).Encode(),
				3: asm0.Set(SetDestPins, 1).Delay(1).Encode(),
				4: asm0.Set(SetDestPins, 0).Encode(),
				5: asm0.Jmp(JmpXNZeroDec, 3).Encode(),
			},
			expectprog: []uint16{
				//     .wrap_target
				0xe081, //  0: set    pindirs, 1
				0x80a0, //  1: pull   block
				0xa027, //  2: mov    x, osr
				0xe101, //  3: set    pins, 1                [1]
				0xe000, //  4: set    pins, 0
				0x0043, //  5: jmp    x--, 3
				//     .wrap
			},
		},
		{
			name: "spi3w",
			program: []uint16{
				//     .wrap_target
				// LOOP: write out x-1 bits.
				0: asm1.Out(OutDestPins, 1).Side(0).Encode(),    //  0: out    pins, 1         side 0
				1: asm1.Jmp(JmpXNZeroDec, 0).Side(1).Encode(),   //  1: jmp    x--, 0          side 1
				2: asm1.Jmp(JmpYZero, 7).Side(0).Encode(),       //  2: jmp    !y, 7           side 0
				3: asm1.Set(SetDestPindirs, 0).Side(0).Encode(), //  3: set    pindirs, 0      side 0
				4: asm1.Nop().Side(0).Encode(),                  //  4: nop                    side 0
				// LOOP: read in y-1 bits.
				5: asm1.In(InSrcPins, 1).Side(1).Encode(),     // 5: in     pins, 1         side 1
				6: asm1.Jmp(JmpYNZeroDec, 5).Side(0).Encode(), //  6: jmp    y--, 5          side 0
				// LOOP: Wait for SPI packet on IRQ.
				7: asm1.WaitPin(true, 0).Side(0).Encode(), //  7: wait   1 pin, 0        side 0
				8: asm1.IRQSet(false, 0).Side(0).Encode(), //  8: irq    nowait 0        side 0
			},
			expectprog: []uint16{
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
			},
		},
		{
			name: "i2s",
			program: []uint16{
				//     .wrap_target
				0: asm2.Out(OutDestPins, 1).Side(2).Encode(),  //  0: out    pins, 1         side 2
				1: asm2.Jmp(JmpXNZeroDec, 0).Side(3).Encode(), //  1: jmp    x--, 0          side 3
				2: asm2.Out(OutDestPins, 1).Side(0).Encode(),  //  2: out    pins, 1         side 0
				3: asm2.Set(SetDestX, 14).Side(1).Encode(),    //  3: set    x, 14           side 1
				4: asm2.Out(OutDestPins, 1).Side(0).Encode(),  //  4: out    pins, 1         side 0
				5: asm2.Jmp(JmpXNZeroDec, 4).Side(1).Encode(), //  5: jmp    x--, 4          side 1
				6: asm2.Out(OutDestPins, 1).Side(2).Encode(),  //  6: out    pins, 1         side 2
				7: asm2.Set(SetDestX, 14).Side(3).Encode(),    //  7: set    x, 14           side 3
			},
			expectprog: []uint16{
				//     .wrap_target
				0x7001, //  0: out    pins, 1         side 2
				0x1840, //  1: jmp    x--, 0          side 3
				0x6001, //  2: out    pins, 1         side 0
				0xe82e, //  3: set    x, 14           side 1
				0x6001, //  4: out    pins, 1         side 0
				0x0844, //  5: jmp    x--, 4          side 1
				0x7001, //  6: out    pins, 1         side 2
				0xf82e, //  7: set    x, 14           side 3
				//     .wrap
			},
		},
		{
			name: "spi_cpha0",
			program: []uint16{
				//     .wrap_target
				0: asm1.Out(OutDestPins, 1).Side(0).Delay(1).Encode(), //  0: out    pins, 1         side 0 [1]
				1: asm1.In(InSrcPins, 1).Side(1).Delay(1).Encode(),    //  1: in     pins, 1         side 1 [1]
				//     .wrap
			},
			expectprog: []uint16{
				//     .wrap_target
				0x6101, //  0: out    pins, 1         side 0 [1]
				0x5101, //  1: in     pins, 1         side 1 [1]
				//     .wrap
			},
		},
		{
			name: "spi_cpha1",
			program: []uint16{
				//     .wrap_target
				0: asm1.Out(OutDestX, 1).Side(0).Encode(),                   //  0: out    x, 1            side 0
				1: asm1.Mov(MovDestPins, MovSrcX).Side(1).Delay(1).Encode(), //  1: mov    pins, x         side 1 [1]
				2: asm1.In(InSrcPins, 1).Side(0).Encode(),                   //  2: in     pins, 1         side 0
				//     .wrap
			},
			expectprog: []uint16{
				//     .wrap_target
				0x6021, //  0: out    x, 1            side 0
				0xb101, //  1: mov    pins, x         side 1 [1]
				0x4001, //  2: in     pins, 1         side 0
				//     .wrap
			},
		},
		{
			name: "ws2812b",
			program: []uint16{
				//     .wrap_target
				0: asm0.Pull(true, true).Encode(),                //  0: pull   ifempty block
				1: asm0.Set(SetDestPins, 1).Encode(),             //  1: set    pins, 1
				2: asm0.Out(OutDestY, 1).Encode(),                //  2: out    y, 1
				3: asm0.Jmp(JmpYZero, 5).Encode(),                //  3: jmp    !y, 5
				4: asm0.Jmp(JmpAlways, 6).Delay(2).Encode(),      //  4: jmp    6                      [2]
				5: asm0.Set(SetDestPins, 0).Delay(2).Encode(),    //  5: set    pins, 0                [2]
				6: asm0.Set(SetDestPins, 0).Encode(),             //  6: set    pins, 0
				7: asm0.Jmp(JmpOSRNotEmpty, 1).Delay(1).Encode(), //  7: jmp    !osre, 1               [1]
				//     .wrap
			},
			expectprog: []uint16{
				//     .wrap_target
				0x80e0, //  0: pull   ifempty block
				0xe001, //  1: set    pins, 1
				0x6041, //  2: out    y, 1
				0x0065, //  3: jmp    !y, 5
				0x0206, //  4: jmp    6                      [2]
				0xe200, //  5: set    pins, 0                [2]
				0xe000, //  6: set    pins, 0
				0x01e1, //  7: jmp    !osre, 1               [1]
				//     .wrap
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.program) != len(test.expectprog) {
				t.Fatal("mismatched program length")
			}
			for i, got := range test.program {
				want := test.expectprog[i]
				if got != want {
					t.Errorf("mismatched program instruction @%d want %02x, got %02x", i, want, got)
				}
			}
		})
	}
}

func TestAssemblerV1(t *testing.T) {
	asm1 := AssemblerV1{SidesetBits: 1}
	var tests = []struct {
		name       string
		program    []uint16
		expectprog []uint16
	}{
		{
			name: "ws2812bfourpixels",
			program: []uint16{
				//     .wrap_target
				0: asm1.MovOSRFromRx(false, 0).Side(0).Encode(),          // 0: mov    osr, rxfifo[y]  side 0
				1: asm1.Out(OutDestX, 1).Side(0).Delay(2).Encode(),       // 1: out    x, 1            side 0 [2]
				2: asm1.Jmp(JmpXZero, 4).Side(1).Delay(1).Encode(),       // 2: jmp    !x, 4           side 1 [1]
				3: asm1.Jmp(JmpOSRNotEmpty, 1).Side(1).Delay(4).Encode(), // 3: jmp    !osre, 1        side 1 [4]
				4: asm1.Jmp(JmpOSRNotEmpty, 1).Side(0).Delay(4).Encode(), // 4: jmp    !osre, 1        side 0 [4]
				5: asm1.Jmp(JmpYNZeroDec, 0).Side(0).Encode(),            // 5: jmp    y--, 0          side 0
				6: asm1.Set(SetDestX, 31).Side(0).Delay(15).Encode(),     // 6: set    x, 31           side 0 [15]
				7: asm1.Set(SetDestY, 3).Side(0).Delay(15).Encode(),      // 7: set    y, 3            side 0 [15]
				8: asm1.Jmp(JmpXNZeroDec, 7).Side(0).Delay(15).Encode(),  // 8: jmp    x--, 7          side 0 [15]
				//     .wrap
			},
			expectprog: []uint16{
				//     .wrap_target
				0x8090, //  0: mov    osr, rxfifo[y]  side 0
				0x6221, //  1: out    x, 1            side 0 [2]
				0x1124, //  2: jmp    !x, 4           side 1 [1]
				0x14e1, //  3: jmp    !osre, 1        side 1 [4]
				0x04e1, //  4: jmp    !osre, 1        side 0 [4]
				0x0080, //  5: jmp    y--, 0          side 0
				0xef3f, //  6: set    x, 31           side 0 [15]
				0xef43, //  7: set    y, 3            side 0 [15]
				0x0f47, //  8: jmp    x--, 7          side 0 [15]
				//     .wrap
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.program) != len(test.expectprog) {
				t.Fatal("mismatched program length")
			}
			for i, got := range test.program {
				want := test.expectprog[i]
				if got != want {
					t.Errorf("mismatched program instruction @%d want %02x, got %02x", i, want, got)
				}
			}
		})
	}
}
