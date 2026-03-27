// Package tdf provides a parser and renderer for TDF (TheDraw Font) files.
// TDF is a binary font format from the BBS era used to create ANSI block-letter
// art. Each font file contains a fixed 18-byte magic header followed by one or
// more font records, each defining ANSI art for ASCII characters 32–126.
package tdf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Magic is the TDF file header prefix (18 bytes, without the trailing 0x1a).
const Magic = "\x13TheDraw FONTS FILE"

// ErrNotTDF is returned when a file is not a valid TDF font.
var ErrNotTDF = errors.New("not a valid TDF font file")

// Font represents a parsed TDF font.
type Font struct {
	Name      string
	Type      byte   // 0=Color, 1=Block, 2=Outline
	Spacing   byte
	BlockSize uint16
	// chars maps ASCII code (32-126) → raw block bytes (blocksize*2 bytes)
	chars map[byte][]byte
}

// Parse parses a TDF font file from raw bytes.
// Returns ErrNotTDF if the magic bytes don't match.
func Parse(data []byte) (*Font, error) {
	if len(data) < len(Magic)+1 {
		return nil, ErrNotTDF
	}
	if string(data[:len(Magic)]) != Magic {
		return nil, ErrNotTDF
	}
	// Skip magic + 0x1a byte
	pos := len(Magic) + 1
	if pos >= len(data) {
		return nil, fmt.Errorf("tdf: truncated after magic")
	}
	// Read first font record
	f := &Font{chars: make(map[byte][]byte)}
	// Font name: 13 bytes null-terminated
	if pos+13 > len(data) {
		return nil, fmt.Errorf("tdf: truncated at name")
	}
	nameBytes := make([]byte, 13)
	copy(nameBytes, data[pos:pos+13])
	for i, b := range nameBytes {
		if b == 0 {
			nameBytes = nameBytes[:i]
			break
		}
	}
	f.Name = string(nameBytes)
	pos += 13
	// Font type, spacing, block size
	if pos+4 > len(data) {
		return nil, fmt.Errorf("tdf: truncated at metadata")
	}
	f.Type = data[pos]
	pos++
	f.Spacing = data[pos]
	pos++
	f.BlockSize = uint16(data[pos]) | uint16(data[pos+1])<<8
	pos += 2
	// Read 95 character blocks (ASCII 32–126)
	chunkSize := int(f.BlockSize) * 2
	if chunkSize <= 0 {
		// BlockSize of 0 means no character data — return font with empty char map.
		return f, nil
	}
	for ascii := byte(32); ascii <= 126; ascii++ {
		end := pos + chunkSize
		if end > len(data) {
			break
		}
		block := make([]byte, chunkSize)
		copy(block, data[pos:end])
		f.chars[ascii] = block
		pos += chunkSize
	}
	return f, nil
}

// MeasureWidth returns the total visible width of text when rendered with this font.
func (f *Font) MeasureWidth(text string) int {
	height := 8
	charWidth := int(f.BlockSize) / height
	if charWidth <= 0 {
		charWidth = 1
	}
	return len([]rune(text)) * (charWidth + int(f.Spacing))
}

// Render renders text as ANSI block-letter art using this font.
// Returns plain text as a fallback if the font width exceeds maxWidth or
// if the font has no character data.
// The rendered string may contain ANSI escape sequences.
func (f *Font) Render(text string, maxWidth int) (string, error) {
	if len(text) == 0 {
		return "", nil
	}
	// For a TDF color font, each character renders as a column of cells.
	// BlockSize = total cells per character (width_cells * height_rows).
	// Height is typically 8-16 rows for most BBS fonts.
	// We assume height = 8 (most common BBS font height).
	height := 8
	charWidth := int(f.BlockSize) / height
	if charWidth <= 0 {
		charWidth = 1
	}
	// Estimate rendered width — fall back to plain text if too wide.
	if f.MeasureWidth(text) > maxWidth {
		return text, nil
	}
	// If no character data is available (e.g. placeholder font), fall back.
	if len(f.chars) == 0 {
		return text, nil
	}
	// DOS color to ANSI RGB mapping (16 DOS colors).
	dosColors := []string{
		"#000000", "#0000aa", "#00aa00", "#00aaaa",
		"#aa0000", "#aa00aa", "#aa5500", "#aaaaaa",
		"#555555", "#5555ff", "#55ff55", "#55ffff",
		"#ff5555", "#ff55ff", "#ffff55", "#ffffff",
	}
	rst := "\x1b[0m"
	// Build one strings.Builder per row.
	rows := make([]strings.Builder, height)
	for _, ch := range text {
		ascii := byte(ch)
		block, ok := f.chars[ascii]
		if !ok || len(block) < height*charWidth*2 {
			// Render as spaces for unknown or short-data chars.
			for row := 0; row < height; row++ {
				rows[row].WriteString(strings.Repeat(" ", charWidth))
			}
			continue
		}
		for row := 0; row < height; row++ {
			for col := 0; col < charWidth; col++ {
				idx := (col*height + row) * 2
				if idx+1 >= len(block) {
					break
				}
				charByte := block[idx]
				attrByte := block[idx+1]
				fg := attrByte & 0x0f
				bg := (attrByte >> 4) & 0x07
				var fgEsc, bgEsc string
				if int(fg) < len(dosColors) {
					r2, g2, b2 := parseHexRGB(dosColors[fg])
					fgEsc = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r2, g2, b2)
				}
				if int(bg) < len(dosColors) {
					r2, g2, b2 := parseHexRGB(dosColors[bg])
					bgEsc = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r2, g2, b2)
				}
				var cell string
				if charByte == 0 {
					cell = " "
				} else {
					cell = cp437ToUTF8(charByte)
				}
				rows[row].WriteString(fgEsc + bgEsc + cell + rst)
			}
		}
	}
	var sb strings.Builder
	for i, row := range rows {
		sb.WriteString(row.String())
		if i < height-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// cp437ToUTF8 converts a CP437 byte to its UTF-8 Unicode equivalent.
// This is a partial table covering common BBS block/line-drawing characters.
func cp437ToUTF8(b byte) string {
	table := map[byte]string{
		0xB0: "░", 0xB1: "▒", 0xB2: "▓", 0xDB: "█",
		0xDC: "▄", 0xDD: "▌", 0xDE: "▐", 0xDF: "▀",
		0xC4: "─", 0xCD: "═", 0xB3: "│", 0xBA: "║",
		0xDA: "┌", 0xBF: "┐", 0xC0: "└", 0xD9: "┘",
		0xC9: "╔", 0xBB: "╗", 0xC8: "╚", 0xBC: "╝",
		0x04: "♦", 0x05: "♣", 0x06: "♠",
		0x01: "☺", 0x02: "☻",
	}
	if s, ok := table[b]; ok {
		return s
	}
	if b >= 32 && b < 127 {
		return string(rune(b))
	}
	return "·" // fallback for unmapped bytes
}

// parseHexRGB parses a "#rrggbb" hex color string into R, G, B components.
func parseHexRGB(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	parse := func(s string) uint8 {
		v, _ := strconv.ParseUint(s, 16, 8)
		return uint8(v)
	}
	return parse(hex[0:2]), parse(hex[2:4]), parse(hex[4:6])
}
