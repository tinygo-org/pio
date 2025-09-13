package main

import (
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

//go:generate pioasm -o go rxfifoputget.pio rxfifoputget_pio.go

/*
RxFIFOPutGet is a non-functional example purely for documentation purposes.

It demonstrates how to enable the new pio.FifoJoinRxPutGet mode, enabling
both of the new FJOIN_RX_GET and FJOIN_RX_PUT modes. This allows the PIO
to use the RX FIFO for both reading and writing data at any desired index,
at the cost of denying all access to the RX FIFO from the system.
*/
func main() {
	sm, _ := pio.PIO0.ClaimStateMachine()
	putget, err := NewRxFIFOPutGet(sm)
	if err != nil {
		panic(err.Error())
	}

	putget.Enable(true)

	for {
		time.Sleep(5 * time.Second)
	}
}

type RxFIFOPutGet struct {
	sm            pio.StateMachine
	offsetPlusOne uint8
}

func NewRxFIFOPutGet(sm pio.StateMachine) (*RxFIFOPutGet, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	Pio := sm.PIO()

	// rxfifoputgetInstructions and rxfifoputgetOrigin are auto-generated in rxfifoputget_pio.go
	// by the pioasm tool based on the .program field in the PIO assembly code
	offset, err := Pio.AddProgram(rxfifoputgetInstructions, rxfifoputgetOrigin)
	if err != nil {
		return nil, err
	}
	cfg := rxfifoputgetProgramDefaultConfig(offset)
	// Enable FJOIN_RX_PUT and FJOIN_RX_GET mode in the state machine. This mode prevents access
	// to the RX FIFO from the system, but gives the PIO four extra registers.
	cfg.SetFIFOJoin(pio.FifoJoinRxPutGet)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	return &RxFIFOPutGet{sm: sm, offsetPlusOne: offset + 1}, nil
}

func (r *RxFIFOPutGet) Enable(enabled bool) {
	r.sm.SetEnabled(enabled)
}
