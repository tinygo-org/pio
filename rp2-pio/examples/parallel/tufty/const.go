package main

const (
	CASET    byte = 0x2A
	COLMOD   byte = 0x3A
	DISPOFF  byte = 0x28
	DISPON   byte = 0x29
	FRCTRL2  byte = 0xC6
	GAMSET   byte = 0x26
	GCTRL    byte = 0xB7
	GMCTRN1  byte = 0xE1
	GMCTRP1  byte = 0xE0
	INVOFF   byte = 0x20
	INVON    byte = 0x21
	LCMCTRL  byte = 0xC0
	MADCTL   byte = 0x36
	NORON    byte = 0x13
	PORCTRL  byte = 0xB2
	PWCTRL1  byte = 0xD0
	PWMFRSEL byte = 0xCC
	RAMWR    byte = 0x2C
	RASET    byte = 0x2B
	SLPOUT   byte = 0x11
	SWRESET  byte = 0x01
	TEOFF    byte = 0x34
	TEON     byte = 0x35
	VCOMS    byte = 0xBB
	VDVS     byte = 0xC4
	VDVVRHEN byte = 0xC2
	VRHS     byte = 0xC3
	RAMCTRL  byte = 0xB0
)

const (
	ROW_ORDER   uint8 = 0b10000000
	COL_ORDER   uint8 = 0b01000000
	SWAP_XY     uint8 = 0b00100000 // AKA "MV"
	SCAN_ORDER  uint8 = 0b00010000
	RGB_BGR     uint8 = 0b00001000
	HORIZ_ORDER uint8 = 0b00000100
)
