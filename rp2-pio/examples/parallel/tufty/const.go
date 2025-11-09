package main

const (
	SWRESET  byte = 0x01
	TEOFF    byte = 0x34
	TEON     byte = 0x35
	MADCTL   byte = 0x36
	COLMOD   byte = 0x3A
	GCTRL    byte = 0xB7
	VCOMS    byte = 0xBB
	LCMCTRL  byte = 0xC0
	VDVVRHEN byte = 0xC2
	VRHS     byte = 0xC3
	VDVS     byte = 0xC4
	FRCTRL2  byte = 0xC6
	PWCTRL1  byte = 0xD0
	PORCTRL  byte = 0xB2
	GMCTRP1  byte = 0xE0
	GMCTRN1  byte = 0xE1
	INVOFF   byte = 0x20
	SLPOUT   byte = 0x11
	DISPON   byte = 0x29
	GAMSET   byte = 0x26
	DISPOFF  byte = 0x28
	RAMWR    byte = 0x2C
	INVON    byte = 0x21
	CASET    byte = 0x2A
	RASET    byte = 0x2B
	PWMFRSEL byte = 0xCC
)

const (
	ROW_ORDER   uint8 = 0b10000000
	COL_ORDER   uint8 = 0b01000000
	SWAP_XY     uint8 = 0b00100000 // AKA "MV"
	SCAN_ORDER  uint8 = 0b00010000
	RGB_BGR     uint8 = 0b00001000
	HORIZ_ORDER uint8 = 0b00000100
)
