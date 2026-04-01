// Quick debug tool — prints the rune values in the inertia font for "GL1TCH"
package main

import (
	"fmt"
	"github.com/8op-org/gl1tch/internal/tdf"
)

func main() {
	f, _ := tdf.LoadEmbedded("inertia")
	fmt.Printf("Font height: %d  spacing: %d\n", f.Height, f.Spacing)
	for _, r := range "GL1TCH" {
		g, ok := f.Glyph(r)
		if !ok {
			fmt.Printf("%c: no glyph\n", r)
			continue
		}
		fmt.Printf("\n%c  width=%d height=%d\n", r, g.Width, g.Height)
		for row := 0; row < int(g.Height); row++ {
			for col := 0; col < int(g.Width); col++ {
				cell := g.Cells[row*int(g.Width)+col]
				fmt.Printf("  [%d,%d] ch=U+%04X %q color=0x%02x\n",
					row, col, cell.Ch, string(cell.Ch), cell.Color)
			}
		}
	}
}
