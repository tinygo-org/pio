// this example uses the rp2040 I2S peripheral to play a sine wave.
// It uses a PCM5102A DAC to convert the digital signal to analog.
// The sine wave is played at 44.1 kHz.
// The sine wave is played in blocks of 32 samples.
// The sine wave is played for 500 ms, then paused for 500 ms.
// The sine wave is played indefinitely.
// The sine wave is played on both the left and right channels.
// connect PCM5102A DIN to GPIO2
// connect PCM5102A BCK to GPIO3
// connect PCM5102A LCK to GPIO4
package main

import (
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

const (
	i2sDataPin  = machine.GPIO2
	i2sClockPin = machine.GPIO3

	NUM_SAMPLES = 32
	NUM_BLOCKS  = 5
)

// sine wave data
var sine []int16 = []int16{
	6392, 12539, 18204, 23169, 27244, 30272, 32137, 32767, 32137,
	30272, 27244, 23169, 18204, 12539, 6392, 0, -6393, -12540,
	-18205, -23170, -27245, -30273, -32138, -32767, -32138, -30273, -27245,
	-23170, -18205, -12540, -6393, -1,
}

func main() {
	time.Sleep(time.Millisecond * 500)

	sm, _ := pio.PIO0.ClaimStateMachine()
	i2s, err := piolib.NewI2S(sm, i2sDataPin, i2sClockPin)
	if err != nil {
		panic(err.Error())
	}

	i2s.SetSampleFrequency(44100)

	data := make([]uint32, NUM_SAMPLES*NUM_BLOCKS)
	for i := 0; i < NUM_SAMPLES*NUM_BLOCKS; i++ {
		data[i] = uint32(sine[i%NUM_SAMPLES]) | uint32(sine[i%NUM_SAMPLES])<<16
	}

	// Play the sine wave
	for {
		for i := 0; i < 50; i++ {
			i2s.WriteStereo(data)
		}

		time.Sleep(time.Millisecond * 500)
	}
}
