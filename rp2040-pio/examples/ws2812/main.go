//go:generate pioasm -o go ws2812.pio ws2812_pio.go
package main

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2040-pio"
)

func main() {
	var ws pioWS2812

	err := ws.init(pio.PIO0.StateMachine(0), machine.GP16)
	if err != nil {
		panic(err.Error())
	}
	for r := uint8(255); r > 0; r-- {
		for g := uint8(255); g > 0; g-- {
			for b := uint8(255); b > 0; b-- {
				println("txfifo:", ws.sm.TxFIFOLevel(), "r:", r, "g:", g, "b:", b)
				ws.SetRGB(r, g, b)
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

type pioWS2812 struct {
	sm pio.StateMachine
}

func (ws *pioWS2812) init(sm pio.StateMachine, pin machine.Pin) (err error) {
	// We add the program to PIO memory and store it's offset.
	Pio := sm.PIO()
	offset, err := Pio.AddProgram(ws2812_ledInstructions, ws2812_ledOrigin)
	if err != nil {
		return err
	}
	pin.Configure(machine.PinConfig{Mode: Pio.PinMode()})
	sm.SetConsecutivePinDirs(pin, 1, true)
	cfg := ws2812_ledProgramDefaultConfig(offset)
	cfg.SetOutPins(pin, 1)
	// We only use Tx FIFO, so we set the join to Tx.
	cfg.SetFIFOJoin(pio.FifoJoinTx)
	sm.Init(offset, cfg)
	sm.SetEnabled(true)

	ws.sm = sm
	return nil
}

func (ws *pioWS2812) SetRGB(r, g, b uint8) {
	color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	ws.sm.TxPut(color)
}
