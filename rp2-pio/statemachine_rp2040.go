//go:build rp2040

package pio

import (
	"device/rp"
)

func (sm StateMachine) setConfig(cfg StateMachineConfig) {
	sm.PIO().BlockIndex() // Panic if PIO or state machine not at valid offset.
	if sm.index > 3 {
		panic(badStateMachineIndex)
	}
	hw := sm.HW()
	hw.CLKDIV.Set(cfg.ClkDiv)
	hw.EXECCTRL.Set(cfg.ExecCtrl)
	hw.SHIFTCTRL.Set(cfg.ShiftCtrl)
	hw.PINCTRL.Set(cfg.PinCtrl)
}

func (sm StateMachine) isValid() bool {
	return sm.pio != nil && sm.index <= 3 &&
		(sm.pio.hw == rp.PIO0 || sm.pio.hw == rp.PIO1)
}

func (sm StateMachine) getRxFIFOAt(fifoIndex int) uint32 {
	panic("GetRxFIFOAt not supported on rp2040")
}

func (sm StateMachine) setRxFIFOAt(data uint32, fifoIndex int) {
	panic("SetRxFIFOAt not supported on rp2040")
}
