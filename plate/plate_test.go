package plate

import (
	"strings"
	"testing"

	"github.com/mineracks/seedhammer-v1-companion/engrave/wire/sh1e"
)

func TestTextBlockRoundTripsThroughSH1E(t *testing.T) {
	d := sh1e.Design{PlateType: sh1e.SquarePlate, TextBlocks: []sh1e.TextBlock{
		{FontID: sh1e.FontConstant, Size: 12, XMM: 42, YMM: 40, Alignment: sh1e.AlignCenter, Text: "Mineracks 2026-10-07"},
	}}
	b, err := sh1e.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := sh1e.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Build(d2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Strokes == 0 || r.Commands < r.Strokes {
		t.Fatalf("no strokes: %+v", r)
	}
	// centred on x=42 at a 4 mm face: the text spans ~52 mm, so 16..68
	if f(r.Bounds.Min.X) < 14 || f(r.Bounds.Max.X) > 70 || f(r.Bounds.Min.Y) < 39 || f(r.Bounds.Max.Y) > 45 {
		t.Fatalf("unexpected bounds %.1f,%.1f-%.1f,%.1f", f(r.Bounds.Min.X), f(r.Bounds.Min.Y), f(r.Bounds.Max.X), f(r.Bounds.Max.Y))
	}
}

func TestOutsidePlateIsRefused(t *testing.T) {
	d := sh1e.Design{PlateType: sh1e.SquarePlate, TextBlocks: []sh1e.TextBlock{
		{FontID: sh1e.FontConstant, Size: 60, XMM: 2, YMM: 2, Text: "WWWWWWWW"},
	}}
	if _, err := Build(d); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("want outside-plate error, got %v", err)
	}
}

func TestNearEdgeWarns(t *testing.T) {
	d := sh1e.Design{PlateType: sh1e.SquarePlate, TextBlocks: []sh1e.TextBlock{
		{FontID: sh1e.FontConstant, Size: 9, XMM: 5, YMM: 5, Text: "A"},
	}}
	r, err := Build(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "10 mm") {
		t.Fatalf("want a near-edge warning, got %v", r.Warnings)
	}
}

func TestQRBlockSizeAndStrokes(t *testing.T) {
	q := sh1e.QRBlock{XMM: 10, YMM: 10, ModuleTenths: 9, Level: 1, Data: []byte("hello plate")}
	size, err := QRSizeMM(q)
	if err != nil {
		t.Fatal(err)
	}
	// 11 bytes -> version 1 (21 modules) at 0.9 mm = 18.9 mm
	if size < 18 || size > 20 {
		t.Fatalf("size %.1f", size)
	}
	r, err := Build(sh1e.Design{PlateType: sh1e.SquarePlate, QRBlocks: []sh1e.QRBlock{q}})
	if err != nil {
		t.Fatal(err)
	}
	if f(r.Bounds.Min.X) < 9.5 || f(r.Bounds.Max.X) > 29.5 {
		t.Fatalf("qr bounds %.1f..%.1f", f(r.Bounds.Min.X), f(r.Bounds.Max.X))
	}
}

func TestSVGPathParsesSubsetAndFlattensCurves(t *testing.T) {
	segs, err := parsePath("M0 0 L10 0 h5 v5 Q20 10 15 15 c-5 0 -5 5 0 5 Z")
	if err != nil {
		t.Fatal(err)
	}
	if segs[0].op != 'M' || len(segs) < 3+curveChords*2 {
		t.Fatalf("%d segs", len(segs))
	}
	if _, err := parsePath("M0 0 A5 5 0 0 1 10 10"); err == nil {
		t.Fatal("arcs must be refused")
	}
	r, err := Build(sh1e.Design{PlateType: sh1e.SquarePlate, SvgPaths: []sh1e.SvgPath{{XMM: 20, YMM: 20, ScalePct: 100, PathD: "M0 0 L20 0 L20 20 Z"}}})
	if err != nil {
		t.Fatal(err)
	}
	if f(r.Bounds.Min.X) != 20 || f(r.Bounds.Max.X) != 40 || r.Strokes != 3 {
		t.Fatalf("bounds %.1f..%.1f strokes %d", f(r.Bounds.Min.X), f(r.Bounds.Max.X), r.Strokes)
	}
}

func TestShareDesignFitsALargePlate(t *testing.T) {
	// a realistic envelope share: ~410 bytes of CBOR (ciphertext of the descriptor + one SSKR share)
	payload := make([]byte, 410)
	for i := range payload {
		payload[i] = byte(i*7 + 3)
	}
	a, b, err := ShareDesign(Share{Title: "Family vault", UrType: "envelope", Fingerprint: "c9ad277c", K: 2, N: 3, Index: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.QRBlocks) != 1 || a.QRBlocks[0].ModuleTenths != 9 {
		t.Fatalf("side A qr %+v", a.QRBlocks)
	}
	for name, d := range map[string]sh1e.Design{"A": a, "B": b} {
		enc, err := sh1e.Encode(d)
		if err != nil {
			t.Fatalf("side %s encode: %v", name, err)
		}
		d2, err := sh1e.Decode(enc)
		if err != nil {
			t.Fatalf("side %s decode: %v", name, err)
		}
		r, err := Build(d2)
		if err != nil {
			t.Fatalf("side %s build: %v", name, err)
		}
		t.Logf("side %s: %d strokes, %d commands, bounds (%.1f,%.1f)-(%.1f,%.1f) mm, %d text blocks, sh1e %d bytes, warnings %v",
			name, r.Strokes, r.Commands, f(r.Bounds.Min.X), f(r.Bounds.Min.Y), f(r.Bounds.Max.X), f(r.Bounds.Max.Y), len(d.TextBlocks), len(enc), r.Warnings)
		if len(r.Warnings) != 0 {
			t.Fatalf("side %s: %v", name, r.Warnings)
		}
		if strings.Count(Preview(r), "L") < 100 {
			t.Fatal("preview has no strokes")
		}
	}
	back, err := ParseShareText(strings.ToLower(b.TextBlocks[3].Text + "\n" + b.TextBlocks[4].Text + b.TextBlocks[5].Text))
	if err != nil || string(back) != string(payload) {
		t.Fatalf("text side does not round-trip: %v (%d bytes)", err, len(back))
	}
}
