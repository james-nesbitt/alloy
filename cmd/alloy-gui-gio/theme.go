package main

import (
	"image/color"

	"gioui.org/widget/material"
)

func applySystemTheme(th *material.Theme) {
	// High-contrast Dark Mode (XDG Generic Dark)
	th.Palette = material.Palette{
		Fg:         color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // White text
		Bg:         color.NRGBA{R: 15, G: 15, B: 15, A: 255},    // Deep black bg
		ContrastFg: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // White text on buttons
		ContrastBg: color.NRGBA{R: 0, G: 100, B: 200, A: 255},   // Strong Blue accents
	}
}
