// Package pio provides PIO implementation for TinyGo
package pio

// PIO represents a PIO interface
type PIO struct {
    enabled bool
}

// New creates a new PIO instance
func New() *PIO {
    return &PIO{enabled: true}
}

// IsEnabled returns whether PIO is enabled
func (p *PIO) IsEnabled() bool {
    return p.enabled
}
