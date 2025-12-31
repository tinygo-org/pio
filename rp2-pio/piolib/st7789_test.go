package piolib

import (
	"testing"
)

func TestNewST7789(t *testing.T) {
	display := NewST7789(240, 320)
	if display == nil {
		t.Fatal("Failed to create display")
	}
}

func TestWriteCommand(t *testing.T) {
	display := NewST7789(240, 320)
	display.WriteCommand(0x01)
}

func TestSetWindow(t *testing.T) {
	display := NewST7789(240, 320)
	display.SetWindow(0, 0, 239, 319)
}
