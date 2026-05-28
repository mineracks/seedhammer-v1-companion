package sh1e

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
	"testing"
)

// ─── Fixtures ─────────────────────────────────────────────────────────────
//
// Fixtures are constructed via factory functions, not shared globals,
// because tests mutate them and slice headers share backing arrays —
// mutating one would pollute others. Each call returns an independent
// Design value.

// minimal returns the smallest interesting design: a single text block on
// a SmallPlate.
func minimal() Design {
	return Design{
		PlateType: SmallPlate,
		TextBlocks: []TextBlock{{
			FontID:    FontComfortaa,
			Size:      12,
			XMM:       5,
			YMM:       5,
			Alignment: AlignLeft,
			Text:      "ABANDON ABILITY ABLE ABOUT ABOVE ABSENT ABSORB",
		}},
	}
}

// withLogo returns the spec § Example 2 — title + seed words + BTC logo.
func withLogo() Design {
	return Design{
		PlateType: SquarePlate,
		TextBlocks: []TextBlock{
			{FontID: FontPoppins, Size: 18, XMM: 10, YMM: 8, Alignment: AlignCenter, Text: "MY MULTISIG KEY 1 OF 3"},
			{FontID: FontComfortaa, Size: 12, XMM: 5, YMM: 25, Alignment: AlignLeft, Text: strings.Repeat("X", 200)},
		},
		SvgPaths: []SvgPath{
			{XMM: 70, YMM: 5, ScalePct: 100, PathD: "M 0 0 L 10 0 L 10 10 L 0 10 Z M 2 2 L 8 8 M 8 2 L 2 8"},
		},
	}
}

// ─── Round-trip ───────────────────────────────────────────────────────────

func TestEncodeDecode_Minimal_Roundtrip(t *testing.T) {
	bytes1, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(bytes1)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Fingerprint will be populated; the input had a zero fingerprint, so
	// compare on the substantive fields plus assert fingerprint was set.
	want := minimal()
	want.Fingerprint = got.Fingerprint // copy through
	if !equalDesign(got, want) {
		t.Errorf("round-trip mismatch\n got: %+v\nwant: %+v", got, want)
	}
	if got.Fingerprint == ([32]byte{}) {
		t.Errorf("Fingerprint not set after round-trip")
	}
}

func TestEncodeDecode_WithLogo_Roundtrip(t *testing.T) {
	b, err := Encode(withLogo())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.PlateType != SquarePlate {
		t.Errorf("PlateType: got %d want %d", got.PlateType, SquarePlate)
	}
	if len(got.TextBlocks) != 2 {
		t.Errorf("TextBlocks len: got %d want 2", len(got.TextBlocks))
	}
	if len(got.SvgPaths) != 1 {
		t.Errorf("SvgPaths len: got %d want 1", len(got.SvgPaths))
	}
}

// ─── Canonical encoding stability ─────────────────────────────────────────

func TestEncode_StableBytes(t *testing.T) {
	// Same input must produce byte-identical output across encodes.
	b1, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode 1: %v", err)
	}
	b2, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode 2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("Encode of same input produced different bytes\n  b1: %x\n  b2: %x", b1, b2)
	}
}

func TestEncode_FingerprintIndependentOfInput(t *testing.T) {
	// Two designs identical apart from Fingerprint must encode identically.
	d1 := minimal() // Fingerprint zero
	d2 := minimal()
	d2.Fingerprint = [32]byte{0xff, 0xee, 0xdd} // bogus — encoder must overwrite

	b1, err := Encode(d1)
	if err != nil {
		t.Fatalf("Encode d1: %v", err)
	}
	b2, err := Encode(d2)
	if err != nil {
		t.Fatalf("Encode d2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("Encode must overwrite Fingerprint; bytes differ")
	}
}

// ─── Envelope rejections ──────────────────────────────────────────────────

func TestDecode_TruncatedBeforeHeader(t *testing.T) {
	if _, err := Decode([]byte{0x53, 0x48, 0x31}); !errors.Is(err, ErrTruncated) {
		t.Errorf("want ErrTruncated, got %v", err)
	}
}

func TestDecode_BadMagic(t *testing.T) {
	good, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bad := bytes.Clone(good)
	bad[0] = 'X' // corrupt magic
	if _, err := Decode(bad); !errors.Is(err, ErrBadMagic) {
		t.Errorf("want ErrBadMagic, got %v", err)
	}
}

func TestDecode_UnsupportedVersion(t *testing.T) {
	good, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bad := bytes.Clone(good)
	bad[4] = 0x99 // bump version
	if _, err := Decode(bad); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("want ErrUnsupportedVersion, got %v", err)
	}
}

func TestDecode_LengthMismatch(t *testing.T) {
	good, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bad := bytes.Clone(good)
	// Inflate the declared length so it exceeds actual payload.
	binary.LittleEndian.PutUint16(bad[5:7], uint16(len(good)-envelopeOverhead+10))
	if _, err := Decode(bad); !errors.Is(err, ErrLengthMismatch) {
		t.Errorf("want ErrLengthMismatch, got %v", err)
	}
}

func TestDecode_CRCMismatch(t *testing.T) {
	good, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	bad := bytes.Clone(good)
	bad[envelopeOverhead] ^= 0x01 // flip a bit in the payload
	if _, err := Decode(bad); !errors.Is(err, ErrCRCMismatch) {
		t.Errorf("want ErrCRCMismatch, got %v", err)
	}
}

func TestDecode_FingerprintMismatch(t *testing.T) {
	// Encode a design, then forge a copy whose CBOR payload has a bogus
	// fingerprint but a recomputed CRC over the bogus payload. The Decode
	// should still reject because the recomputed fingerprint won't match.
	good, err := Encode(minimal())
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode, mutate fingerprint, re-encode with the canonical encoder
	// to produce a syntactically valid but fingerprint-wrong payload.
	d, err := Decode(good)
	if err != nil {
		t.Fatalf("Decode 1: %v", err)
	}
	d.Fingerprint = [32]byte{0xde, 0xad, 0xbe, 0xef}
	bogusPayload, err := canonicalEncMode.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal bogus: %v", err)
	}
	// Manually assemble envelope with correct CRC over the bogus payload.
	bad := append([]byte{}, Magic[:]...)
	bad = append(bad, Version)
	bad = binary.LittleEndian.AppendUint16(bad, uint16(len(bogusPayload)))
	// Use the real crc32 helper instead of hardcoding.
	bad = binary.LittleEndian.AppendUint32(bad, hashCRC(bogusPayload))
	bad = append(bad, bogusPayload...)

	if _, err := Decode(bad); !errors.Is(err, ErrFingerprintMismatch) {
		t.Errorf("want ErrFingerprintMismatch, got %v", err)
	}
}

// hashCRC is a tiny helper that recomputes CRC32 in tests; mirrors the
// production codec's use of hash/crc32 IEEE so the tests don't drift if
// the impl ever swaps tables.
func hashCRC(b []byte) uint32 {
	return crc32.ChecksumIEEE(b)
}

// ─── Validation rejections ────────────────────────────────────────────────

func TestEncode_BadPlateType(t *testing.T) {
	d := minimal()
	d.PlateType = 99
	if _, err := Encode(d); !errors.Is(err, ErrInvalidEnum) {
		t.Errorf("want ErrInvalidEnum, got %v", err)
	}
}

func TestEncode_BadFontID(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].FontID = 42
	if _, err := Encode(d); !errors.Is(err, ErrInvalidEnum) {
		t.Errorf("want ErrInvalidEnum, got %v", err)
	}
}

func TestEncode_FontSizeOutOfRange(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].Size = 0
	if _, err := Encode(d); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("Size=0: want ErrOutOfRange, got %v", err)
	}
	d.TextBlocks[0].Size = 999
	if _, err := Encode(d); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("Size=999: want ErrOutOfRange, got %v", err)
	}
}

func TestEncode_TextTooLong(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].Text = strings.Repeat("X", MaxTextBytes+1)
	if _, err := Encode(d); !errors.Is(err, ErrTextTooLong) {
		t.Errorf("want ErrTextTooLong, got %v", err)
	}
}

func TestEncode_NonASCII(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].Text = "café" // 'é' is 2 UTF-8 bytes both ≥ 0x80
	if _, err := Encode(d); !errors.Is(err, ErrNonASCII) {
		t.Errorf("want ErrNonASCII, got %v", err)
	}
}

func TestEncode_TooManyBlocks(t *testing.T) {
	d := minimal()
	d.TextBlocks = make([]TextBlock, MaxTextBlocks+1)
	for i := range d.TextBlocks {
		d.TextBlocks[i] = TextBlock{FontID: FontComfortaa, Size: 12, Text: "x"}
	}
	if _, err := Encode(d); !errors.Is(err, ErrTooManyBlocks) {
		t.Errorf("want ErrTooManyBlocks, got %v", err)
	}
}

func TestEncode_TooManyPaths(t *testing.T) {
	d := minimal()
	d.SvgPaths = make([]SvgPath, MaxSvgPaths+1)
	for i := range d.SvgPaths {
		d.SvgPaths[i] = SvgPath{ScalePct: 100, PathD: "M 0 0 L 1 1"}
	}
	if _, err := Encode(d); !errors.Is(err, ErrTooManyPaths) {
		t.Errorf("want ErrTooManyPaths, got %v", err)
	}
}

func TestEncode_PathTooLong(t *testing.T) {
	d := minimal()
	d.SvgPaths = []SvgPath{{
		ScalePct: 100,
		PathD:    strings.Repeat("M 0 0 ", MaxSvgPathDLength), // grossly exceeds
	}}
	if _, err := Encode(d); !errors.Is(err, ErrPathTooLong) {
		t.Errorf("want ErrPathTooLong, got %v", err)
	}
}

func TestEncode_BadAlignment(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].Alignment = 9
	if _, err := Encode(d); !errors.Is(err, ErrInvalidEnum) {
		t.Errorf("want ErrInvalidEnum, got %v", err)
	}
}

func TestEncode_BadRotation(t *testing.T) {
	d := minimal()
	d.TextBlocks[0].Rotation = 45 // only 0/90/180/270 allowed
	if _, err := Encode(d); !errors.Is(err, ErrInvalidRotation) {
		t.Errorf("want ErrInvalidRotation, got %v", err)
	}
}

func TestEncode_BadScalePct(t *testing.T) {
	d := minimal()
	d.SvgPaths = []SvgPath{{ScalePct: 0, PathD: "M 0 0"}}
	if _, err := Encode(d); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("ScalePct=0: want ErrOutOfRange, got %v", err)
	}
	d.SvgPaths[0].ScalePct = MaxScalePercent + 1
	if _, err := Encode(d); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("ScalePct>max: want ErrOutOfRange, got %v", err)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func equalDesign(a, b Design) bool {
	if a.PlateType != b.PlateType {
		return false
	}
	if len(a.TextBlocks) != len(b.TextBlocks) {
		return false
	}
	for i := range a.TextBlocks {
		if a.TextBlocks[i] != b.TextBlocks[i] {
			return false
		}
	}
	if len(a.SvgPaths) != len(b.SvgPaths) {
		return false
	}
	for i := range a.SvgPaths {
		if a.SvgPaths[i] != b.SvgPaths[i] {
			return false
		}
	}
	return a.Fingerprint == b.Fingerprint
}
