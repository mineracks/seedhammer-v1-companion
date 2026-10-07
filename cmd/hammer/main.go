// Command hammer drives a real SeedHammer v1 engraver (MarkingWay, USB serial) from a
// computer, using the same mjolnir driver the Pi Zero firmware uses.
//
//	go run ./cmd/hammer -dev /dev/cu.usbserial-A100U6EB -plate square -outline        # pen-up trace of the plate's engrave area
//	go run ./cmd/hammer -dev ... -plate square -text "HELLO" -dry                        # pen-up trace of a text layout
//	go run ./cmd/hammer -dev ... -plate square -text "HELLO"                             # engrave it (hammer down)
//
// Without -dry the needle hammers the plate. There is no undo.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mineracks/seedhammer-v1-companion/driver/mjolnir"
	"github.com/mineracks/seedhammer-v1-companion/engrave"
	"github.com/mineracks/seedhammer-v1-companion/font/constant"
	"github.com/mineracks/seedhammer-v1-companion/font/vector"
)

// Margins, from upstream backup/backup.go. outerMargin is the dashed line -outline traces;
// nuts and bolts hold the plate at its edges, so nothing may be engraved outside it. Text is
// kept a further innerMargin in, as upstream does, and the Large plate's middle holes are
// avoided by confining content to the 85 mm band 24.5 mm down (upstream's "Avoid the middle
// holes" offset).
const (
	outerMarginMM = 3
	innerMarginMM = 10
	largeBandTop  = 24.5
	largeBandH    = 85
)

var plates = map[string]image.Point{
	"small":  image.Pt(85, 55),
	"square": image.Pt(85, 85),
	"large":  image.Pt(85, 134),
}

// jig is where each plate's top-left corner sits, in mm from the machine's home position.
// Lifted from upstream v1.3.0 cmd/controller/platform_rpi.go (engraver.Engrave): x = 97 for
// every plate, y = 49 for the Square plate so it sits centred in the Large slot, y = 0 for
// Large. The driver package never knew this -- it lives in the Pi platform code -- which is
// why a plan run "from home" lands on the wrong part of the bed. Small (85x55) is not an
// upstream v1.3 size; give -offset for it.
var jig = map[string]image.Point{
	"square": image.Pt(97, 49),
	"large":  image.Pt(97, 0),
}

func main() {
	dev := flag.String("dev", "/dev/cu.usbserial-A100U6EB", "serial device")
	plate := flag.String("plate", "square", "plate: small, square, large")
	outline := flag.Bool("outline", false, "trace the engrave area's perimeter (always pen-up)")
	text := flag.String("text", "", "text to engrave (\\n for a new line)")
	em := flag.Int("em", 6, "text size in mm")
	dry := flag.Bool("dry", false, "pen up throughout: trace the layout without engraving")
	visit := flag.String("visit", "", "calibration: pen-up visit to a list of x,y mm points from home, e.g. 0,0;85,0;85,85;0,85")
	offset := flag.String("offset", "", "override the plate's position as x,y mm from home (default: upstream jig table)")
	dwell := flag.Int("dwell", 5, "seconds to hold at each -visit point")
	flag.Parse()

	if *visit != "" {
		visitPoints(*dev, *visit, *dwell)
		return
	}

	dims, ok := plates[strings.ToLower(*plate)]
	if !ok {
		log.Fatalf("unknown plate %q", *plate)
	}
	mm := mjolnir.Params.Millimeter
	origin, ok := jig[strings.ToLower(*plate)]
	if *offset != "" {
		var ox, oy float64
		if _, err := fmt.Sscanf(*offset, "%f,%f", &ox, &oy); err != nil {
			log.Fatalf("bad -offset %q (want x,y in mm)", *offset)
		}
		origin, ok = image.Pt(int(ox), int(oy)), true
	}
	if !ok {
		log.Fatalf("no jig position known for plate %q: give -offset x,y (mm from home)", *plate)
	}
	area := image.Rectangle{
		Min: image.Pt(outerMarginMM*mm, outerMarginMM*mm),
		Max: dims.Mul(mm).Sub(image.Pt(outerMarginMM*mm, outerMarginMM*mm)),
	}

	// where text may go: innerMargin in from every edge; on Large, only the 85 mm band
	// between the middle holes and the end holes
	textArea := area.Inset((innerMarginMM - outerMarginMM) * mm)
	if strings.ToLower(*plate) == "large" {
		textArea.Min.Y = int(largeBandTop*float64(mm)) + innerMarginMM*mm
		textArea.Max.Y = int((largeBandTop+largeBandH)*float64(mm)) - innerMarginMM*mm
	}

	var plan engrave.Plan
	switch {
	case *outline:
		plan = engrave.Rect(area).Engrave
		*dry = true
	case *text != "":
		plan = textPlan(strings.ReplaceAll(*text, `\n`, "\n"), *em*mm, textArea)
	default:
		log.Fatal("give -outline or -text")
	}

	b := engrave.Measure(plan) // measured BEFORE DryRun: Measure only sees pen-down segments
	if *dry {
		plan = engrave.DryRun(plan)
	}
	n := 0
	plan(func(engrave.Command) { n++ })
	fmt.Printf("plate %s %dx%d mm at jig (%d,%d) mm, plan: %d commands, bounds on plate (%.1f,%.1f)-(%.1f,%.1f) mm, pen %s\n",
		*plate, dims.X, dims.Y, origin.X, origin.Y, n, fmm(b.Min.X, mm), fmm(b.Min.Y, mm), fmm(b.Max.X, mm), fmm(b.Max.Y, mm),
		map[bool]string{true: "UP (dry run)", false: "DOWN (engraving)"}[*dry])
	safe := area
	if !*outline {
		safe = textArea
	}
	if !b.In(safe) {
		log.Fatalf("plan leaves the plate's safe area (%.1f,%.1f)-(%.1f,%.1f) mm -- nuts and bolts live outside it", fmm(safe.Min.X, mm), fmm(safe.Min.Y, mm), fmm(safe.Max.X, mm), fmm(safe.Max.Y, mm))
	}
	// plate coordinates -> bed coordinates, exactly as the Pi firmware does it
	plan = engrave.Offset(origin.X*mm, origin.Y*mm, plan)

	port, err := mjolnir.Open(*dev)
	if err != nil {
		log.Fatalf("open %s: %v", *dev, err)
	}
	defer port.Close()

	quit := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; fmt.Println("cancelling..."); close(quit) }()

	if err := mjolnir.Engrave(port, mjolnir.Options{}, plan, quit); err != nil {
		log.Fatalf("engrave: %v", err)
	}
	fmt.Println("done")
}

func fmm(steps, mm int) float64 { return float64(steps) / float64(mm) }

// visitPoints parks the head at each point in turn (pen up) so a human can see where the
// controller's coordinates land on the bed. Each stop is its own Engrave call, so the head
// re-homes before every move -- slower, but every position is measured from home.
func visitPoints(dev, list string, dwell int) {
	port, err := mjolnir.Open(dev)
	if err != nil {
		log.Fatalf("open %s: %v", dev, err)
	}
	defer port.Close()
	mm := mjolnir.Params.Millimeter
	for _, pt := range strings.Split(list, ";") {
		var x, y float64
		if _, err := fmt.Sscanf(strings.TrimSpace(pt), "%f,%f", &x, &y); err != nil {
			log.Fatalf("bad point %q (want x,y in mm)", pt)
		}
		p := image.Pt(int(x*float64(mm)), int(y*float64(mm)))
		fmt.Printf("-> (%.0f, %.0f) mm from home ... ", x, y)
		empty := engrave.Plan(func(func(engrave.Command)) {})
		if err := mjolnir.Engrave(port, mjolnir.Options{End: p}, empty, nil); err != nil {
			log.Fatalf("move: %v", err)
		}
		fmt.Printf("holding %ds\n", dwell)
		time.Sleep(time.Duration(dwell) * time.Second)
	}
	fmt.Println("done (head left at the last point; it re-homes on the next run)")
}

// face is the one outline font the v1 engraver punches from (the LCD bitmap faces are not strokes).
func face() *vector.Face { return constant.Font }

func textPlan(txt string, em int, area image.Rectangle) engrave.Plan {
	// The engraver's outline font is ASCII-only (uppercase, digits, a little punctuation).
	// Drop what it cannot draw and say so, rather than panicking mid-plan.
	f := face()
	var kept, dropped []rune
	for _, r := range txt {
		if r == '\n' {
			kept = append(kept, r)
			continue
		}
		if _, _, ok := f.Decode(r); ok {
			kept = append(kept, r)
		} else {
			dropped = append(dropped, r)
		}
	}
	if len(dropped) > 0 {
		fmt.Printf("note: the engraver's font has no glyph for %q -- left out\n", string(dropped))
	}
	s := engrave.String(f, em, string(kept))
	sz := s.Measure()
	// centred in the safe area
	off := area.Min.Add(area.Size().Sub(sz).Div(2))
	return engrave.Offset(off.X, off.Y, s.Engrave)
}
