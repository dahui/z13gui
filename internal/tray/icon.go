// Copyright 2026 Jeff Hagadorn
// SPDX-License-Identifier: Apache-2.0

package tray

import (
	"image"
	"image/color"
	"strings"
)

// glyphs is a 5x7 bitmap font for the letters and digits used in tray badges.
// Each row uses the low five bits, leftmost pixel in bit 4.
var glyphs = map[rune][7]uint8{
	'A': {0b01110, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'B': {0b11110, 0b10001, 0b10001, 0b11110, 0b10001, 0b10001, 0b11110},
	'C': {0b01111, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b01111},
	'D': {0b11110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b11110},
	'E': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b11111},
	'F': {0b11111, 0b10000, 0b10000, 0b11110, 0b10000, 0b10000, 0b10000},
	'G': {0b01111, 0b10000, 0b10000, 0b10111, 0b10001, 0b10001, 0b01110},
	'H': {0b10001, 0b10001, 0b10001, 0b11111, 0b10001, 0b10001, 0b10001},
	'I': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b11111},
	'J': {0b00111, 0b00010, 0b00010, 0b00010, 0b00010, 0b10010, 0b01100},
	'K': {0b10001, 0b10010, 0b10100, 0b11000, 0b10100, 0b10010, 0b10001},
	'L': {0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b10000, 0b11111},
	'M': {0b10001, 0b11011, 0b10101, 0b10101, 0b10001, 0b10001, 0b10001},
	'N': {0b10001, 0b11001, 0b10101, 0b10011, 0b10001, 0b10001, 0b10001},
	'O': {0b01110, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'P': {0b11110, 0b10001, 0b10001, 0b11110, 0b10000, 0b10000, 0b10000},
	'Q': {0b01110, 0b10001, 0b10001, 0b10001, 0b10101, 0b10010, 0b01101},
	'R': {0b11110, 0b10001, 0b10001, 0b11110, 0b10100, 0b10010, 0b10001},
	'S': {0b01111, 0b10000, 0b10000, 0b01110, 0b00001, 0b00001, 0b11110},
	'T': {0b11111, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100, 0b00100},
	'U': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01110},
	'V': {0b10001, 0b10001, 0b10001, 0b10001, 0b10001, 0b01010, 0b00100},
	'W': {0b10001, 0b10001, 0b10001, 0b10101, 0b10101, 0b11011, 0b10001},
	'X': {0b10001, 0b10001, 0b01010, 0b00100, 0b01010, 0b10001, 0b10001},
	'Y': {0b10001, 0b10001, 0b01010, 0b00100, 0b00100, 0b00100, 0b00100},
	'Z': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b10000, 0b11111},
	'0': {0b01110, 0b10001, 0b10011, 0b10101, 0b11001, 0b10001, 0b01110},
	'1': {0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b11111},
	'2': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b01000, 0b11111},
	'3': {0b11110, 0b00001, 0b00001, 0b01110, 0b00001, 0b00001, 0b11110},
	'4': {0b00010, 0b00110, 0b01010, 0b10010, 0b11111, 0b00010, 0b00010},
	'5': {0b11111, 0b10000, 0b10000, 0b11110, 0b00001, 0b00001, 0b11110},
	'6': {0b01110, 0b10000, 0b10000, 0b11110, 0b10001, 0b10001, 0b01110},
	'7': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	'8': {0b01110, 0b10001, 0b10001, 0b01110, 0b10001, 0b10001, 0b01110},
	'9': {0b01110, 0b10001, 0b10001, 0b01111, 0b00001, 0b00001, 0b01110},
	'?': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b00000, 0b00100},
}

// profileIcon returns a 32x32 badge with the first letter of profile on a
// colour that identifies the profile.
func profileIcon(profile string) image.Image {
	const (
		size  = 32
		scale = 3
	)
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := profileColor(profile)
	centre := float64(size-1) / 2
	radius := float64(size)/2 - 0.5
	for y := range size {
		for x := range size {
			dx := float64(x) - centre
			dy := float64(y) - centre
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, bg)
			}
		}
	}

	glyph, ok := glyphs[profileLetter(profile)]
	if !ok {
		glyph = glyphs['?']
	}
	const (
		glyphW = 5
		glyphH = 7
	)
	originX := (size - glyphW*scale) / 2
	originY := (size - glyphH*scale) / 2
	white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	for row, bits := range glyph {
		for col := range glyphW {
			if bits&(1<<(glyphW-1-col)) == 0 {
				continue
			}
			for dy := range scale {
				for dx := range scale {
					img.SetRGBA(originX+col*scale+dx, originY+row*scale+dy, white)
				}
			}
		}
	}

	return img
}

func profileLetter(profile string) rune {
	if profile == "" {
		return '?'
	}
	r := []rune(strings.ToUpper(profile))[0]
	switch {
	case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return r
	default:
		return '?'
	}
}

func profileColor(profile string) color.RGBA {
	switch profile {
	case "quiet":
		return color.RGBA{R: 0x2d, G: 0x9c, B: 0xdb, A: 0xff}
	case "balanced":
		return color.RGBA{R: 0xe0, G: 0xa0, B: 0x30, A: 0xff}
	case "performance":
		return color.RGBA{R: 0xe0, G: 0x52, B: 0x52, A: 0xff}
	case "custom":
		return color.RGBA{R: 0x8b, G: 0x5c, B: 0xf6, A: 0xff}
	default:
		return color.RGBA{R: 0x6b, G: 0x72, B: 0x80, A: 0xff}
	}
}
