// Command sh1e-decode validates and pretty-prints an SH1E envelope.
//
// Intended for Pi-side use during real-device validation: the v1
// controller scans a QR code, hands the bytes to its own embedded
// decoder, and shows a preview on the LCD. This helper does the same
// validation chain on stdin or a file so you can replay payloads from
// the host machine — useful for fuzz-test triage, regression testing
// against captured QR scans, and ad-hoc "does this byte string parse?"
// questions.
//
// Usage:
//
//	sh1e-decode < some.sh1e          # decode from stdin
//	sh1e-decode some.sh1e            # decode from path
//	sh1e-decode -hex "53 48 31 45…"  # decode from a hex dump
//
// Output is human-readable: envelope header, validation result, each
// text block's content, each SVG path's preview. Non-zero exit on any
// parse / validation failure (errors.Is-mapped) so it integrates with
// CI fuzzers.
package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sh1e-decode:", err)
		os.Exit(1)
	}
}

func run() error {
	hexInput := flag.String("hex", "", "decode from a hex-encoded string (spaces/newlines OK) instead of stdin/file")
	flag.Parse()

	var raw []byte
	switch {
	case *hexInput != "":
		cleaned := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\t' || r == ':' || r == '|' {
				return -1
			}
			return r
		}, *hexInput)
		b, err := hex.DecodeString(cleaned)
		if err != nil {
			return fmt.Errorf("hex decode: %w", err)
		}
		raw = b
	case flag.NArg() == 1:
		b, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			return err
		}
		raw = b
	default:
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		raw = b
	}

	design, err := sh1e.Decode(raw)
	if err != nil {
		// Tag known error types so CI can grep their names.
		tag := "OTHER"
		switch {
		case errors.Is(err, sh1e.ErrBadMagic):
			tag = "BAD_MAGIC"
		case errors.Is(err, sh1e.ErrUnsupportedVersion):
			tag = "UNSUPPORTED_VERSION"
		case errors.Is(err, sh1e.ErrLengthMismatch):
			tag = "LENGTH_MISMATCH"
		case errors.Is(err, sh1e.ErrCRCMismatch):
			tag = "CRC_MISMATCH"
		case errors.Is(err, sh1e.ErrBadCBOR):
			tag = "BAD_CBOR"
		case errors.Is(err, sh1e.ErrNotCanonical):
			tag = "NOT_CANONICAL"
		case errors.Is(err, sh1e.ErrTruncated):
			tag = "TRUNCATED"
		case errors.Is(err, sh1e.ErrFingerprintMismatch):
			tag = "FINGERPRINT_MISMATCH"
		case errors.Is(err, sh1e.ErrTooManyBlocks):
			tag = "TOO_MANY_BLOCKS"
		case errors.Is(err, sh1e.ErrTooManyPaths):
			tag = "TOO_MANY_PATHS"
		case errors.Is(err, sh1e.ErrTextTooLong):
			tag = "TEXT_TOO_LONG"
		case errors.Is(err, sh1e.ErrPathTooLong):
			tag = "PATH_TOO_LONG"
		case errors.Is(err, sh1e.ErrNonASCII):
			tag = "NON_ASCII"
		case errors.Is(err, sh1e.ErrInvalidEnum):
			tag = "INVALID_ENUM"
		case errors.Is(err, sh1e.ErrOutOfRange):
			tag = "OUT_OF_RANGE"
		case errors.Is(err, sh1e.ErrInvalidRotation):
			tag = "INVALID_ROTATION"
		}
		return fmt.Errorf("REJECT %s: %w", tag, err)
	}

	prettyPrint(os.Stdout, raw, design)
	return nil
}

func prettyPrint(w io.Writer, raw []byte, d sh1e.Design) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "ACCEPT — %d bytes, version 0x%02x\n", len(raw), sh1e.Version)
	fmt.Fprintf(&buf, "plate:       %s\n", plateName(d.PlateType))
	fmt.Fprintf(&buf, "fingerprint: %x\n", d.Fingerprint)
	fmt.Fprintf(&buf, "text blocks: %d\n", len(d.TextBlocks))
	for i, tb := range d.TextBlocks {
		fmt.Fprintf(&buf,
			"  [%2d] font=%s size=%dpt anchor=(%d,%d) align=%s rot=%d  %q\n",
			i, fontName(tb.FontID), tb.Size, tb.XMM, tb.YMM,
			alignName(tb.Alignment), tb.Rotation, tb.Text,
		)
	}
	fmt.Fprintf(&buf, "svg paths:   %d\n", len(d.SvgPaths))
	for i, p := range d.SvgPaths {
		preview := p.PathD
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		fmt.Fprintf(&buf,
			"  [%2d] anchor=(%d,%d) scale=%d%% rot=%d  d=%q\n",
			i, p.XMM, p.YMM, p.ScalePct, p.Rotation, preview,
		)
	}
	_, _ = w.Write(buf.Bytes())
}

func plateName(p sh1e.PlateType) string {
	switch p {
	case sh1e.SmallPlate:
		return "Small (SH-01, 85×55mm)"
	case sh1e.SquarePlate:
		return "Square (SH-02, 85×85mm)"
	case sh1e.LargePlate:
		return "Large (SH-03, 85×134mm)"
	}
	return fmt.Sprintf("unknown plate %d", p)
}

func fontName(f sh1e.FontID) string {
	switch f {
	case sh1e.FontComfortaa:
		return "Comfortaa"
	case sh1e.FontPoppins:
		return "Poppins"
	case sh1e.FontConstant:
		return "Constant"
	}
	return fmt.Sprintf("unknown-%d", f)
}

func alignName(a sh1e.Alignment) string {
	switch a {
	case sh1e.AlignLeft:
		return "Left"
	case sh1e.AlignCenter:
		return "Center"
	case sh1e.AlignRight:
		return "Right"
	}
	return fmt.Sprintf("unknown-%d", a)
}
