package main

import (
	"fmt"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
)

//go:generate pioasm -o go rxfifoput.pio rxfifoput_pio.go

/*
RxFIFOPut is a simple example of a PIO counter demonstrating how to use the new
PIO FJOIN_RX_PUT mode introduced with the RP2350.

RxFIFOPut increments a counter in three PIO cycles, or every 20 nanoseconds. It
is not possible for the system to read from the RX FIFO fast enough to keep up
with the PIO at this speed, so the RX FIFO is used as a status buffer to store
the current counter value.

In order to read from the RX FIFO at a specific index, the system uses the new
StateMachine.GetRxFIFOAt() method.
*/
func main() {
	sm, _ := pio.PIO0.ClaimStateMachine()
	put, err := NewRxFIFOPut(sm)
	if err != nil {
		panic(err.Error())
	}

	for {
		fmt.Printf("FIFO: %08x %08x %08x %08x\r\n", put.Get(0), put.Get(1), put.Get(2), put.Get(3))
		time.Sleep(5 * time.Second)
	}
}

type RxFIFOPut struct {
	sm            pio.StateMachine
	offsetPlusOne uint8
}

func NewRxFIFOPut(sm pio.StateMachine) (*RxFIFOPut, error) {
	sm.TryClaim() // SM should be claimed beforehand, we just guarantee it's claimed.
	Pio := sm.PIO()

	// rxfifoputInstructions and rxfifoputOrigin are auto-generated in rxfifoput_pio.go
	// by the pioasm tool based on the .program field in the PIO assembly code
	offset, err := Pio.AddProgram(rxfifoputInstructions, rxfifoputOrigin)
	if err != nil {
		return nil, err
	}
	cfg := rxfifoputProgramDefaultConfig(offset)
	// Enable FJOIN_RX_PUT mode
	cfg.SetFIFOJoin(pio.FifoJoinRxPut)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	return &RxFIFOPut{sm: sm, offsetPlusOne: offset + 1}, nil
}

func (r *RxFIFOPut) Enable(enabled bool) {
	r.sm.SetEnabled(enabled)
}

func (r *RxFIFOPut) Get(index int) uint32 {
	return r.sm.GetRxFIFOAt(index)
}
