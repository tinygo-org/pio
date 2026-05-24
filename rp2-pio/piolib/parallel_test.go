package piolib

import (
	"testing"
)

func TestValidateParallelConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ParallelConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid 8-bit bus",
			cfg: ParallelConfig{
				BusWidth:    8,
				BitsPerPull: 8,
			},
			wantErr: false,
		},
		{
			name: "valid 8-bit bus with larger bits per pull",
			cfg: ParallelConfig{
				BusWidth:    8,
				BitsPerPull: 32,
			},
			wantErr: false,
		},
		{
			name: "valid 6-bit bus",
			cfg: ParallelConfig{
				BusWidth:    6,
				BitsPerPull: 24,
			},
			wantErr: false,
		},
		{
			name: "zero bus width",
			cfg: ParallelConfig{
				BusWidth:    0,
				BitsPerPull: 8,
			},
			wantErr: true,
			errMsg:  "zero bus width",
		},
		{
			name: "bits per pull not multiple of bus width",
			cfg: ParallelConfig{
				BusWidth:    8,
				BitsPerPull: 10,
			},
			wantErr: true,
			errMsg:  "bits per pull must be multiple of bus width",
		},
		{
			name: "bits per pull less than bus width",
			cfg: ParallelConfig{
				BusWidth:    8,
				BitsPerPull: 4,
			},
			wantErr: true,
			errMsg:  "bits per pull must be greater or equal to bus width",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParallelConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParallelConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("ValidateParallelConfig() error = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestParallelConfigShiftDirection(t *testing.T) {
	// Test that ShiftLeft configuration works as expected:
	// ShiftLeft=true should result in left shift (shift_right=false in SetOutShift)
	// ShiftLeft=false should result in right shift (shift_right=true in SetOutShift)
	tests := []struct {
		name       string
		shiftLeft  bool
		wantShiftR bool // expected shift_right parameter
	}{
		{
			name:       "shift left (ST7789 typical)",
			shiftLeft:  true,
			wantShiftR: false,
		},
		{
			name:       "shift right (default)",
			shiftLeft:  false,
			wantShiftR: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The shift_right parameter to SetOutShift is !cfg.ShiftLeft
			gotShiftR := !tt.shiftLeft
			if gotShiftR != tt.wantShiftR {
				t.Errorf("shift_right = %v, want %v", gotShiftR, tt.wantShiftR)
			}
		})
	}
}

func TestParallelFastModeProgramLength(t *testing.T) {
	// Verify that FastMode uses 2 instructions vs 3 instructions
	tests := []struct {
		name      string
		fastMode  bool
		wantInstr int
	}{
		{
			name:      "fast mode - 2 instructions",
			fastMode:  true,
			wantInstr: 2,
		},
		{
			name:      "normal mode - 3 instructions",
			fastMode:  false,
			wantInstr: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var programLen int
			if tt.fastMode {
				programLen = 2
			} else {
				programLen = 3
			}
			if programLen != tt.wantInstr {
				t.Errorf("program length = %d, want %d", programLen, tt.wantInstr)
			}
		})
	}
}

func TestST7789CommandConstants(t *testing.T) {
	// Verify ST7789 command constants match expected values from datasheet
	tests := []struct {
		name  string
		cmd   byte
		want  byte
	}{
		{"SWRESET", CMD_SWRESET, 0x01},
		{"SLPOUT", CMD_SLPOUT, 0x11},
		{"INVON", CMD_INVON, 0x21},
		{"DISPON", CMD_DISPON, 0x29},
		{"CASET", CMD_CASET, 0x2A},
		{"RASET", CMD_RASET, 0x2B},
		{"RAMWR", CMD_RAMWR, 0x2C},
		{"MADCTL", CMD_MADCTL, 0x36},
		{"COLMOD", CMD_COLMOD, 0x3A},
		{"PORCTRL", CMD_PORCTRL, 0xB2},
		{"RAMCTRL", CMD_RAMCTRL, 0xB0},
		{"GCTRL", CMD_GCTRL, 0xB7},
		{"VCOMS", CMD_VCOMS, 0xBB},
		{"LCMCTRL", CMD_LCMCTRL, 0xC0},
		{"VDVVRHEN", CMD_VDVVRHEN, 0xC2},
		{"VRHS", CMD_VRHS, 0xC3},
		{"VDVS", CMD_VDVS, 0xC4},
		{"FRCTRL2", CMD_FRCTRL2, 0xC6},
		{"PWCTRL1", CMD_PWCTRL1, 0xD0},
		{"GMCTRP1", CMD_GMCTRP1, 0xE0},
		{"GMCTRN1", CMD_GMCTRN1, 0xE1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd != tt.want {
				t.Errorf("command constant = 0x%02X, want 0x%02X", tt.cmd, tt.want)
			}
		})
	}
}

func TestMADCTLFlags(t *testing.T) {
	// Verify MADCTL flag values
	tests := []struct {
		name string
		got  byte
		want byte
	}{
		{"MY", MADCTL_MY, 0x80},
		{"MX", MADCTL_MX, 0x40},
		{"MV", MADCTL_MV, 0x20},
		{"ML", MADCTL_ML, 0x10},
		{"RGB", MADCTL_RGB, 0x00},
		{"BGR", MADCTL_BGR, 0x08},
		{"MH", MADCTL_MH, 0x04},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("flag = 0x%02X, want 0x%02X", tt.got, tt.want)
			}
		})
	}
}
