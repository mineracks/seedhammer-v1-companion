package plate

import (
	"encoding/base32"
	"fmt"
	"math"
	"strings"

	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
	"github.com/mineracks/seedhammer-v1-companion/font/constant"
)

// A share plate: one Shamir share of a vault's descriptor on a Large (SH-03) plate, both
// sides, the way upstream flips a plate for side B.
//
// Side A: title, "SHARE i OF n - ANY k RECOVER - <fingerprint>", the share as a QR.
// Side B: the same header, then the share as text, so the plate is recoverable by typing
// if the code will not scan.
//
// What goes in the QR and the text is the share's RAW BYTES (the UR payload, i.e. the CBOR
// under "ur:envelope/..."), not its bytewords string. Measured 2026-10-07 at the engraver's
// stroke font: a 410-byte share is 820 bytewords characters, which is 103 mm of 3 mm text
// and a 99 mm QR at 0.9 mm modules - neither fits an 85 mm plate. As raw bytes the QR is
// ~69 mm at 0.9 mm modules (what a Pi camera reads off hammered metal), and as base32 the
// text is 656 characters, 21 lines. The header names the UR type so a reader knows what
// the bytes are; Any Two Keys re-wraps them as ur:<type>/<bytewords> on the way in.
const (
	ShareTitleSizePt = 9 // 3 mm
	ShareTextSizePt  = 9 // 3 mm: 32 characters per line at the stroke font's width
	shareLineGapMM   = 1
	shareQRModuleBig = 9 // tenths of a mm; falls back to 6 when the code would not fit
)

// Share is one share as the layout needs it.
type Share struct {
	Title       string // plate title, up to 18 characters
	UrType      string // e.g. "envelope" - printed so the text can be re-wrapped as a UR
	Fingerprint string // the vault's master fingerprint, for the header
	K, N, Index int    // any K of N, this is share Index (1-based)
	Payload     []byte // the UR payload bytes (CBOR)
}

// ShareText is the text form engraved on side B: RFC 4648 base32, no padding, uppercase.
func ShareText(payload []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(payload)
}

// ParseShareText reverses ShareText; spaces and line breaks are ignored, case too.
func ParseShareText(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.Join(strings.Fields(s), ""))
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
}

// ShareDesign lays one share out as two SH1E designs: side A (QR) and side B (text).
func ShareDesign(s Share) (sideA, sideB sh1e.Design, err error) {
	if len(s.Payload) == 0 {
		return sideA, sideB, fmt.Errorf("empty share")
	}
	dims, _ := Dims(sh1e.LargePlate)
	centre := int16(dims.X / 2)
	header := func() ([]sh1e.TextBlock, int16) {
		y := int16(InnerMarginMM)
		var blocks []sh1e.TextBlock
		if t := strings.TrimSpace(strings.ToUpper(s.Title)); t != "" {
			blocks = append(blocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: ShareTitleSizePt, XMM: centre, YMM: y, Alignment: sh1e.AlignCenter, Text: clip(t, 18)})
			y += 4
		}
		meta := fmt.Sprintf("SHARE %d OF %d - ANY %d RECOVER", s.Index, s.N, s.K)
		blocks = append(blocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: ShareTitleSizePt, XMM: centre, YMM: y, Alignment: sh1e.AlignCenter, Text: meta})
		y += 4
		kind := strings.ToUpper(strings.TrimSpace(s.UrType))
		if kind == "" {
			kind = "BYTES"
		}
		line := "UR:" + kind
		if fp := strings.ToUpper(strings.TrimSpace(s.Fingerprint)); fp != "" {
			line += " - " + fp
		}
		blocks = append(blocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: ShareTitleSizePt, XMM: centre, YMM: y, Alignment: sh1e.AlignCenter, Text: line})
		y += 5
		return blocks, y
	}

	// ---- side A: the QR
	sideA = sh1e.Design{PlateType: sh1e.LargePlate}
	blocks, y := header()
	sideA.TextBlocks = blocks
	// Big modules first; a lower error-correction level before smaller modules, because the
	// text side is the real backup and 0.9 mm modules are what the camera reads. The code
	// stays inside the inner margin: the Large plate's middle bolts sit at mid-height on
	// both edges, and a wider code would run under their nuts.
	maxW := float64(dims.X - 2*InnerMarginMM)
	var q sh1e.QRBlock
	var size float64
	for _, try := range []sh1e.QRBlock{{ModuleTenths: shareQRModuleBig, Level: 1}, {ModuleTenths: shareQRModuleBig, Level: 0}, {ModuleTenths: 6, Level: 1}, {ModuleTenths: 6, Level: 0}} {
		try.Data = s.Payload
		sz, err := QRSizeMM(try)
		if err != nil {
			return sideA, sideB, fmt.Errorf("share does not fit a QR: %w", err)
		}
		q, size = try, sz
		if sz <= maxW {
			break
		}
	}
	if size > maxW {
		return sideA, sideB, fmt.Errorf("share of %d bytes is too large for a plate QR even at 0.6 mm modules (%.0f mm)", len(s.Payload), size)
	}
	q.XMM = int16(math.Round((float64(dims.X) - size) / 2))
	q.YMM = y
	sideA.QRBlocks = []sh1e.QRBlock{q}
	y += int16(math.Ceil(size)) + 3
	sideA.TextBlocks = append(sideA.TextBlocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: ShareTitleSizePt, XMM: centre, YMM: y, Alignment: sh1e.AlignCenter, Text: "TEXT OF THIS SHARE ON THE BACK"})
	if y+4 > int16(dims.Y-InnerMarginMM) {
		return sideA, sideB, fmt.Errorf("side A overflows the plate")
	}

	// ---- side B: the text
	sideB = sh1e.Design{PlateType: sh1e.LargePlate}
	blocks, y = header()
	sideB.TextBlocks = blocks
	text := ShareText(s.Payload)
	cw, _, _ := constant.Font.Decode('W')
	charMM := float64(cw) * float64(ShareTextSizePt) * 0.33 / float64(constant.Font.Metrics().Height)
	perLine := int(maxW / charMM)
	lineH := int16(math.Ceil(float64(ShareTextSizePt)*0.33)) + shareLineGapMM
	// one text block per up-to-256-byte chunk, lines joined with newlines: the block cap
	// is on bytes, and the stroke font draws '\n' as a line break
	var chunk []string
	flush := func() {
		if len(chunk) == 0 {
			return
		}
		sideB.TextBlocks = append(sideB.TextBlocks, sh1e.TextBlock{FontID: sh1e.FontConstant, Size: ShareTextSizePt, XMM: InnerMarginMM, YMM: y, Alignment: sh1e.AlignLeft, Text: strings.Join(chunk, "\n")})
		y += lineH * int16(len(chunk))
		chunk = nil
	}
	for t := text; t != ""; {
		n := perLine
		if n > len(t) {
			n = len(t)
		}
		if len(strings.Join(chunk, "\n"))+n+1 > sh1e.MaxTextBytes {
			flush()
		}
		chunk = append(chunk, t[:n])
		t = t[n:]
	}
	flush()
	if y > int16(dims.Y-InnerMarginMM) {
		return sideA, sideB, fmt.Errorf("share text needs %d mm, the plate has %d: %d bytes is too long for one SH-03", y, dims.Y-InnerMarginMM, len(s.Payload))
	}
	return sideA, sideB, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
