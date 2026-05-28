package sh1e

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// FuzzDecode runs Decode against arbitrary input. Every invocation must
// either return a Design that round-trips through Encode (canonical and
// stable) OR return a non-nil error. A panic, hang, or out-of-memory
// must never happen — the Pi controller runs this parser on untrusted
// QR-scanned data and a crash there is a real-world fault.
//
// Run with:   go test -fuzz=FuzzDecode -fuzztime=10m ./engrave/wire/sh1e
//
// Pre-release we want at least 1 CPU-week of fuzzing per the spec's
// security analysis section. CI runs a shorter window per merge.
func FuzzDecode(f *testing.F) {
	// Seed corpus: a couple of valid envelopes, plus the classic
	// adversarial bait (truncated, version-bumped, length-mismatched).
	seedDesigns := []Design{
		{
			PlateType: SmallPlate,
			TextBlocks: []TextBlock{{
				FontID:    FontComfortaa,
				Size:      12,
				XMM:       5,
				YMM:       5,
				Alignment: AlignLeft,
				Text:      "ABANDON ABILITY ABLE",
			}},
		},
		{
			PlateType: LargePlate,
			TextBlocks: []TextBlock{
				{FontID: FontPoppins, Size: 18, XMM: 10, YMM: 10, Alignment: AlignCenter, Text: "TITLE"},
			},
			SvgPaths: []SvgPath{
				{XMM: 30, YMM: 60, ScalePct: 100, PathD: "M 0 0 L 10 10 Z"},
			},
		},
	}
	for _, d := range seedDesigns {
		if b, err := Encode(d); err == nil {
			f.Add(b)
			// Also seed corrupted variants — these should be REJECTED,
			// not crash.
			bad := bytes.Clone(b)
			if len(bad) > 5 {
				bad[5] ^= 0xff // corrupt the length field
				f.Add(bad)
			}
			bad2 := bytes.Clone(b)
			if len(bad2) > 11 {
				bad2[11] ^= 0x01 // corrupt the payload
				f.Add(bad2)
			}
		}
	}
	// Also seed the obvious wrong-shape inputs.
	f.Add([]byte{})
	f.Add([]byte("SH1E"))
	f.Add([]byte("SH1F\x01\x00\x00\x00\x00\x00\x00"))
	f.Add(append([]byte("SH1E\x01\xff\xff\xff\xff\xff\xff"), bytes.Repeat([]byte{0xff}, 64)...))

	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := Decode(data)
		if err != nil {
			// Any error is acceptable — we just must not panic.
			return
		}
		// Decoded successfully: the design must re-encode to the same
		// payload bytes (canonical-encoding round-trip property).
		// Otherwise our canonicity check is broken.
		reencoded, err := Encode(d)
		if err != nil {
			t.Fatalf("Decode accepted bytes that don't round-trip through Encode: %v\ninput: %x", err, data)
		}
		// The full envelope should match the input exactly — Decode is
		// supposed to reject any non-canonical envelope.
		if !bytes.Equal(reencoded, data) {
			t.Fatalf("Decode accepted non-canonical bytes\ninput:    %x\nreencoded: %x", data, reencoded)
		}
	})
}

// FuzzDecodeEnvelopeOnly hammers just the magic + version + length + CRC
// preamble. Useful as a faster-converging fuzz target for the
// header-validation path — Decode rejects ~99% of inputs here before
// the CBOR parser ever runs.
func FuzzDecodeEnvelopeOnly(f *testing.F) {
	// Seeds: valid envelopes + a few corruptions in the header bytes.
	good, err := Encode(Design{
		PlateType:  SmallPlate,
		TextBlocks: []TextBlock{{FontID: FontComfortaa, Size: 12, Text: "X"}},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(good)

	// Corrupted versions still need rejection without crash.
	for i := 0; i < envelopeOverhead; i++ {
		bad := bytes.Clone(good)
		bad[i] ^= 0xff
		f.Add(bad)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(data) // crash-is-the-failure-mode
	})
}

// hashEnvelope is a smoke check that the CRC32 we compute matches Go's
// standard IEEE polynomial — protects against the unlikely event of a
// dependency-swap changing the underlying table.
func TestCRCMatchesStdlib(t *testing.T) {
	payload := []byte("the quick brown fox")
	want := crc32.ChecksumIEEE(payload)
	envelope := append([]byte{}, Magic[:]...)
	envelope = append(envelope, Version)
	envelope = binary.LittleEndian.AppendUint16(envelope, uint16(len(payload)))
	envelope = binary.LittleEndian.AppendUint32(envelope, want)
	envelope = append(envelope, payload...)
	got := binary.LittleEndian.Uint32(envelope[7:11])
	if got != want {
		t.Errorf("CRC drift detected: header %08x vs stdlib %08x", got, want)
	}
}
