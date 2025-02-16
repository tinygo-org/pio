package piolib

import (
	"device/rp"
	"machine"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

// See https://github.com/raspberrypi/pico-examples/blob/master/pio/hub75/hub75.pio
type Hub75 struct {
	sm        pio.StateMachine
	rgbOffset uint8
	rowOffset uint8
	nRowPins  uint8
	rowBase   machine.Pin
	latchBase machine.Pin
	clock     machine.Pin
	rgbBase   machine.Pin
}

func NewHub75(sm pio.StateMachine, clock, rgbBase, latchBase, rowBase machine.Pin, nRowPins uint8) (*Hub75, error) {
	sm.TryClaim()
	Pio := sm.PIO()

	rgbOffset, err := Pio.AddProgram(hub75_data_rgb888Instructions, hub75_data_rgb888Origin)
	if err != nil {
		return nil, err
	}
	rowOffset, err := Pio.AddProgram(hub75_rowInstructions, hub75_rowOrigin)
	if err != nil {
		Pio.ClearProgramSection(rgbOffset, uint8(len(hub75_data_rgb888Instructions)))
		return nil, err
	}

	hub := Hub75{
		sm:        sm,
		rgbOffset: rgbOffset,
		rowOffset: rowOffset,
		nRowPins:  uint8(nRowPins),
		rowBase:   rowBase,
		latchBase: latchBase,
	}
	return &hub, nil
}

func (hub *Hub75) initRowProgram() {
	cfgPindirsConsecutive(hub.sm, hub.rowBase, hub.nRowPins, true)
	cfgPindirsConsecutive(hub.sm, hub.latchBase, 2, true)

	cfg := hub75_rowProgramDefaultConfig(hub.rowOffset)
	cfg.SetOutPins(hub.rowBase, hub.nRowPins)
	cfg.SetSidesetPins(hub.latchBase)
	cfg.SetOutShift(true, true, 32)
	hub.sm.Init(hub.rowOffset, cfg)
	hub.sm.SetEnabled(true)
}

func (hub *Hub75) waitTxStall() {
	Pio := hub.sm.PIO()
	txstallmask := 1 << (hub.sm.StateMachineIndex() + rp.PIO0_FDEBUG_TXSTALL_Pos)
	fdebug := &Pio.HW().FDEBUG
	fdebug.Set(uint32(txstallmask))
	for fdebug.Get()&uint32(txstallmask) == 0 {
		gosched()
	}
}

func (hub *Hub75) initRGBProgram() {
	cfgPindirsConsecutive(hub.sm, hub.rgbBase, 6, true)
	cfgPindirsConsecutive(hub.sm, hub.clock, 1, true)

	cfg := hub75_data_rgb888ProgramDefaultConfig(hub.rgbOffset)
	cfg.SetOutPins(hub.rgbBase, 6)
	cfg.SetSidesetPins(hub.clock)
	cfg.SetOutShift(true, true, 24)
	// ISR shift to left. R0 ends up at bit 5. We push it up to MSB and then flip the register.
	cfg.SetInShift(false, false, 32)
	cfg.SetFIFOJoin(pio.FifoJoinTx)

	hub.sm.Init(hub.rgbOffset, cfg)
	// What? This line does not make sense? Executing a position? Exec takes a instruction not an offset!?!
	hub.sm.Exec(uint16(hub.rgbOffset) + hub75_data_rgb888offset_entry_point)
	hub.sm.SetEnabled(true)
}

// rgbSetShift patches a data program at `offset` to preshift pixels by `shamt`
func (hub *Hub75) rgbSetShift(offset, shamt uint8) {
	var instr uint16
	if shamt == 0 {
		instr = pio.EncodePull(false, true) // Blocking pull.
	} else {
		instr = pio.EncodeOut(pio.SrcDestNull, shamt)
	}
	mem := &(hub.sm.PIO().HW().INSTR_MEM)
	mem[offset+hub75_data_rgb888offset_shift0].Set(uint32(instr))
	mem[offset+hub75_data_rgb888offset_shift1].Set(uint32(instr))
}

func cfgPindirsConsecutive(sm pio.StateMachine, base machine.Pin, nPins uint8, output bool) {
	sm.SetPindirsConsecutive(base, nPins, output)
	Pio := sm.PIO()
	pinmode := Pio.PinMode()
	for p := base; p < base+machine.Pin(nPins); p++ {
		p.Configure(machine.PinConfig{Mode: pinmode})
	}
}
