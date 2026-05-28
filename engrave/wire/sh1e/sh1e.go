// Package sh1e implements the v1 SH1E plate-design envelope.
//
// See docs/architecture/sh1e-spec.md for the canonical specification.
// This file is the reference implementation.
package sh1e

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"

	"github.com/fxamacker/cbor/v2"
)

// ─── Envelope constants ───────────────────────────────────────────────────

// Magic bytes "SH1E" prepended to every envelope.
var Magic = [4]byte{'S', 'H', '1', 'E'}

// Version is the current envelope version. Bump on any breaking change.
const Version uint8 = 0x01

// envelopeOverhead is the fixed-size header before the CBOR payload:
// 4 magic + 1 version + 2 payload_len + 4 crc32 = 11.
const envelopeOverhead = 4 + 1 + 2 + 4

// MaxPayloadBytes caps the CBOR payload at 2^16 - 1 (uint16 length field).
const MaxPayloadBytes = 65535

// ─── Domain limits (from spec § Validation) ───────────────────────────────

const (
	MaxTextBlocks      = 32
	MaxTextBytes       = 256
	MaxSvgPaths        = 16
	MaxSvgPathDLength  = 4096
	MinFontSizePoints  = 1
	MaxFontSizePoints  = 200
	MinScalePercent    = 1
	MaxScalePercent    = 1000
)

// ─── Public enums ─────────────────────────────────────────────────────────

// PlateType matches upstream backup.PlateSize indexing (see
// docs/architecture/sh1e-spec.md § Plate types).
type PlateType uint8

const (
	SmallPlate  PlateType = 0
	SquarePlate PlateType = 1
	LargePlate  PlateType = 2
)

func (p PlateType) valid() bool { return p <= LargePlate }

// FontID identifies one of the engraver fonts baked into the v1 firmware.
type FontID uint8

const (
	FontComfortaa FontID = 0
	FontPoppins   FontID = 1
	FontConstant  FontID = 2
)

func (f FontID) valid() bool { return f <= FontConstant }

// Alignment is text-block alignment relative to (XMM, YMM).
type Alignment uint8

const (
	AlignLeft   Alignment = 0
	AlignCenter Alignment = 1
	AlignRight  Alignment = 2
)

func (a Alignment) valid() bool { return a <= AlignRight }

// validRotation accepts 0, 90, 180, 270.
func validRotation(r uint16) bool {
	return r == 0 || r == 90 || r == 180 || r == 270
}

// ─── Public types ─────────────────────────────────────────────────────────

// TextBlock describes one run of text on the plate.
type TextBlock struct {
	FontID    FontID    `cbor:"1,keyasint"`
	Size      uint16    `cbor:"2,keyasint"`
	XMM       int16     `cbor:"3,keyasint"`
	YMM       int16     `cbor:"4,keyasint"`
	Alignment Alignment `cbor:"5,keyasint"`
	Text      string    `cbor:"6,keyasint"`
	Rotation  uint16    `cbor:"7,keyasint,omitempty"`
}

// SvgPath describes a single SVG path placed on the plate.
//
// PathD is the SVG `d` attribute, restricted to the M/L/H/V/Q/C/Z subset
// per spec; arcs and S/T shortcuts are not allowed.
type SvgPath struct {
	XMM      int16  `cbor:"1,keyasint"`
	YMM      int16  `cbor:"2,keyasint"`
	ScalePct uint16 `cbor:"3,keyasint"`
	PathD    string `cbor:"4,keyasint"`
	Rotation uint16 `cbor:"5,keyasint,omitempty"`
}

// Design is the high-level plate intent the composer hands to the Pi.
type Design struct {
	PlateType  PlateType   `cbor:"1,keyasint"`
	TextBlocks []TextBlock `cbor:"2,keyasint"`
	SvgPaths   []SvgPath   `cbor:"3,keyasint,omitempty"`
	// Fingerprint is SHA-256 of the canonical CBOR of fields 1-3 (with
	// field 4 omitted). Set by Encode; verified by Decode.
	Fingerprint [32]byte `cbor:"4,keyasint"`
}

// designForFingerprint is Design without the Fingerprint field — used to
// hash the deterministic-CBOR bytes of fields 1-3 only.
type designForFingerprint struct {
	PlateType  PlateType   `cbor:"1,keyasint"`
	TextBlocks []TextBlock `cbor:"2,keyasint"`
	SvgPaths   []SvgPath   `cbor:"3,keyasint,omitempty"`
}

// ─── Codec ────────────────────────────────────────────────────────────────

var (
	// canonicalEncOpts produces deterministic CBOR per RFC 8949 § 4.2.1:
	// shortest integer form, map keys sorted, no indefinite-length items.
	// Required so the same Design always produces the same envelope bytes
	// (and the same QR). Floats are never emitted — all numerics in our
	// schema are integers — so the float-mode options don't matter.
	canonicalEncOpts = cbor.EncOptions{
		Sort:        cbor.SortBytewiseLexical,
		IndefLength: cbor.IndefLengthForbidden,
	}

	// strictDecMode rejects malformed or non-deterministic input. We do
	// our own additional canonical-encoding check after Unmarshal because
	// fxamacker's strict mode doesn't currently enforce sort order on
	// already-decoded data — we re-encode and compare.
	strictDecOpts = cbor.DecOptions{
		DupMapKey:        cbor.DupMapKeyEnforcedAPF,
		IndefLength:      cbor.IndefLengthForbidden,
		MaxArrayElements: MaxTextBlocks + MaxSvgPaths + 16, // generous slop
		MaxMapPairs:      32,
		MaxNestedLevels:  4,
	}

	canonicalEncMode cbor.EncMode
	strictDecMode    cbor.DecMode
)

func init() {
	em, err := canonicalEncOpts.EncMode()
	if err != nil {
		panic(fmt.Sprintf("sh1e: bad canonical encode options: %v", err))
	}
	canonicalEncMode = em

	dm, err := strictDecOpts.DecMode()
	if err != nil {
		panic(fmt.Sprintf("sh1e: bad strict decode options: %v", err))
	}
	strictDecMode = dm
}

// Encode serialises d into a complete SH1E envelope ready for QR transport.
// The Fingerprint field on d is overwritten with the SHA-256 of the
// canonical CBOR of fields 1-3; pass a zero-value Fingerprint to start.
//
// Returns ErrPayloadTooLarge if the CBOR payload would exceed
// MaxPayloadBytes. Returns other validation errors per Validate.
func Encode(d Design) ([]byte, error) {
	if err := d.validatePreEncode(); err != nil {
		return nil, err
	}
	// Compute the fingerprint over fields 1-3.
	fp := designForFingerprint{
		PlateType:  d.PlateType,
		TextBlocks: d.TextBlocks,
		SvgPaths:   d.SvgPaths,
	}
	fpBytes, err := canonicalEncMode.Marshal(fp)
	if err != nil {
		return nil, fmt.Errorf("sh1e: marshal for fingerprint: %w", err)
	}
	d.Fingerprint = sha256.Sum256(fpBytes)

	// Encode the full envelope payload.
	payload, err := canonicalEncMode.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("sh1e: marshal payload: %w", err)
	}
	if len(payload) > MaxPayloadBytes {
		return nil, fmt.Errorf("%w: payload %d bytes, max %d",
			ErrPayloadTooLarge, len(payload), MaxPayloadBytes)
	}

	out := make([]byte, 0, envelopeOverhead+len(payload))
	out = append(out, Magic[:]...)
	out = append(out, Version)
	out = binary.LittleEndian.AppendUint16(out, uint16(len(payload)))
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(payload))
	out = append(out, payload...)
	return out, nil
}

// Decode parses + validates an SH1E envelope and returns the Design.
//
// All validation rules from docs/architecture/sh1e-spec.md § Parser
// validation rules are applied. The returned Design has Fingerprint set
// from the parsed payload (which Decode verifies matches a recomputed
// hash over fields 1-3).
func Decode(b []byte) (Design, error) {
	if len(b) < envelopeOverhead {
		return Design{}, fmt.Errorf("%w: only %d bytes (need at least %d)",
			ErrTruncated, len(b), envelopeOverhead)
	}
	if [4]byte(b[0:4]) != Magic {
		return Design{}, fmt.Errorf("%w: got %q", ErrBadMagic, b[0:4])
	}
	if b[4] != Version {
		return Design{}, fmt.Errorf("%w: got 0x%02x, supported 0x%02x",
			ErrUnsupportedVersion, b[4], Version)
	}
	payloadLen := binary.LittleEndian.Uint16(b[5:7])
	wantCRC := binary.LittleEndian.Uint32(b[7:11])
	payload := b[11:]
	if int(payloadLen) != len(payload) {
		return Design{}, fmt.Errorf("%w: header says %d, have %d",
			ErrLengthMismatch, payloadLen, len(payload))
	}
	if got := crc32.ChecksumIEEE(payload); got != wantCRC {
		return Design{}, fmt.Errorf("%w: header 0x%08x, computed 0x%08x",
			ErrCRCMismatch, wantCRC, got)
	}

	var d Design
	if err := strictDecMode.Unmarshal(payload, &d); err != nil {
		return Design{}, fmt.Errorf("%w: %v", ErrBadCBOR, err)
	}

	// Reject non-canonical CBOR by round-tripping. If the bytes the
	// caller gave us don't match our canonical re-encoding of the same
	// Design, the input was not canonically encoded.
	if reencoded, err := canonicalEncMode.Marshal(d); err != nil {
		return Design{}, fmt.Errorf("sh1e: reencode for canonicity check: %w", err)
	} else if string(reencoded) != string(payload) {
		return Design{}, ErrNotCanonical
	}

	if err := d.validatePostDecode(); err != nil {
		return Design{}, err
	}

	// Recompute and compare fingerprint.
	fp := designForFingerprint{
		PlateType:  d.PlateType,
		TextBlocks: d.TextBlocks,
		SvgPaths:   d.SvgPaths,
	}
	fpBytes, err := canonicalEncMode.Marshal(fp)
	if err != nil {
		return Design{}, fmt.Errorf("sh1e: marshal for fingerprint check: %w", err)
	}
	if want := sha256.Sum256(fpBytes); want != d.Fingerprint {
		return Design{}, fmt.Errorf("%w: header %x, computed %x",
			ErrFingerprintMismatch, d.Fingerprint, want)
	}

	return d, nil
}

// ─── Validation ───────────────────────────────────────────────────────────

// Sentinel errors. Wrap with fmt.Errorf("%w: …") to add context but keep
// errors.Is-friendly.
var (
	ErrBadMagic            = errors.New("sh1e: bad magic")
	ErrUnsupportedVersion  = errors.New("sh1e: unsupported version")
	ErrLengthMismatch      = errors.New("sh1e: payload length header mismatch")
	ErrCRCMismatch         = errors.New("sh1e: crc32 mismatch")
	ErrBadCBOR             = errors.New("sh1e: malformed cbor payload")
	ErrNotCanonical        = errors.New("sh1e: payload not canonically encoded")
	ErrTruncated           = errors.New("sh1e: truncated envelope")
	ErrFingerprintMismatch = errors.New("sh1e: design fingerprint mismatch")
	ErrPayloadTooLarge     = errors.New("sh1e: payload exceeds max")
	ErrTooManyBlocks       = errors.New("sh1e: too many text blocks")
	ErrTooManyPaths        = errors.New("sh1e: too many svg paths")
	ErrTextTooLong         = errors.New("sh1e: text block too long")
	ErrPathTooLong         = errors.New("sh1e: svg path too long")
	ErrNonASCII            = errors.New("sh1e: non-ascii text (v1 fonts are ascii-only)")
	ErrInvalidEnum         = errors.New("sh1e: invalid enum value")
	ErrOutOfRange          = errors.New("sh1e: numeric field out of range")
	ErrInvalidRotation     = errors.New("sh1e: rotation must be 0, 90, 180, or 270")
)

// validatePreEncode runs the rules that don't depend on the canonical
// encoding having happened yet. Used by Encode.
func (d Design) validatePreEncode() error {
	if !d.PlateType.valid() {
		return fmt.Errorf("%w: plate_type %d", ErrInvalidEnum, d.PlateType)
	}
	if len(d.TextBlocks) > MaxTextBlocks {
		return fmt.Errorf("%w: %d blocks, max %d",
			ErrTooManyBlocks, len(d.TextBlocks), MaxTextBlocks)
	}
	if len(d.SvgPaths) > MaxSvgPaths {
		return fmt.Errorf("%w: %d paths, max %d",
			ErrTooManyPaths, len(d.SvgPaths), MaxSvgPaths)
	}
	for i, tb := range d.TextBlocks {
		if err := tb.validate(); err != nil {
			return fmt.Errorf("text block %d: %w", i, err)
		}
	}
	for i, sp := range d.SvgPaths {
		if err := sp.validate(); err != nil {
			return fmt.Errorf("svg path %d: %w", i, err)
		}
	}
	return nil
}

// validatePostDecode runs the same rules as validatePreEncode after a
// canonical-encoding check has already passed. Kept separate to make the
// decode-time call site explicit even though the rules currently match.
func (d Design) validatePostDecode() error {
	return d.validatePreEncode()
}

func (tb TextBlock) validate() error {
	if !tb.FontID.valid() {
		return fmt.Errorf("%w: font_id %d", ErrInvalidEnum, tb.FontID)
	}
	if tb.Size < MinFontSizePoints || tb.Size > MaxFontSizePoints {
		return fmt.Errorf("%w: size %d (allowed %d-%d)",
			ErrOutOfRange, tb.Size, MinFontSizePoints, MaxFontSizePoints)
	}
	if !tb.Alignment.valid() {
		return fmt.Errorf("%w: alignment %d", ErrInvalidEnum, tb.Alignment)
	}
	if len(tb.Text) > MaxTextBytes {
		return fmt.Errorf("%w: %d bytes, max %d",
			ErrTextTooLong, len(tb.Text), MaxTextBytes)
	}
	for i := 0; i < len(tb.Text); i++ {
		if tb.Text[i] >= 0x80 {
			return fmt.Errorf("%w: byte %d", ErrNonASCII, i)
		}
	}
	if !validRotation(tb.Rotation) {
		return fmt.Errorf("%w: %d", ErrInvalidRotation, tb.Rotation)
	}
	return nil
}

func (sp SvgPath) validate() error {
	if sp.ScalePct < MinScalePercent || sp.ScalePct > MaxScalePercent {
		return fmt.Errorf("%w: scale_pct %d (allowed %d-%d)",
			ErrOutOfRange, sp.ScalePct, MinScalePercent, MaxScalePercent)
	}
	if len(sp.PathD) > MaxSvgPathDLength {
		return fmt.Errorf("%w: %d bytes, max %d",
			ErrPathTooLong, len(sp.PathD), MaxSvgPathDLength)
	}
	if !validRotation(sp.Rotation) {
		return fmt.Errorf("%w: %d", ErrInvalidRotation, sp.Rotation)
	}
	// Strict-subset path syntax (M/L/H/V/Q/C/Z plus lowercase, no arcs,
	// no S/T) is checked by a separate validator that lives alongside the
	// path renderer — out of scope for this package, which is just envelope
	// + structural validation.
	return nil
}
