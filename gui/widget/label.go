package widget

import (
	"image"
	"image/color"
	"math"

	"github.com/mineracks/seedhammer-v1-companion/gui/op"
	"github.com/mineracks/seedhammer-v1-companion/gui/text"
)

func Label(ops op.Ctx, l text.Style, col color.NRGBA, txt string) image.Point {
	return LabelW(ops, l, math.MaxInt, col, txt)
}

func LabelW(ops op.Ctx, l text.Style, width int, col color.NRGBA, txt string) image.Point {
	lines, sz := l.Layout(width, txt)
	for _, line := range lines {
		(&op.TextOp{
			Src:           image.NewUniform(col),
			Face:          l.Face,
			Dot:           image.Pt(line.Dot.X, line.Dot.Y),
			Txt:           line.Text,
			LetterSpacing: l.LetterSpacing,
		}).Add(ops)
	}
	return sz
}
