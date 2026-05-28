// Package bspline provides B-spline curve tessellation + optimisation for
// the engrave pipeline.
//
// Used by the OpenType font rasteriser for glyphs that use B-spline rather
// than Bezier segments, and by the optimiser that finds the smallest
// engrave-stroke representation of a given path.
//
// LIFTED from https://github.com/Gangleri42/seedhammer/tree/seedhammer-features/bspline
// at commit 0a3c63efb125d17d8ec86ce739ecd058c8747cfe. Hardware-agnostic.
// Depends on bezier/ + the gonum linear-programming library.
package bspline
