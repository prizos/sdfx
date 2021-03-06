//-----------------------------------------------------------------------------
/*

Text Operations

Convert a string and font specification into an SDF2

*/
//-----------------------------------------------------------------------------

package sdf

import (
	"io/ioutil"
	"strings"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

//-----------------------------------------------------------------------------

type align int

const (
	L_ALIGN align = iota // left hand side x = 0
	R_ALIGN              // right hand side x = 0
	C_ALIGN              // center x = 0
)

type Text struct {
	s      string
	halign align
}

//-----------------------------------------------------------------------------

// convert a truetype point to a V2
func p_to_V2(p truetype.Point) V2 {
	return V2{float64(p.X), float64(p.Y)}
}

//-----------------------------------------------------------------------------

// return the SDF2 for the n-th curve of the glyph
func glyph_curve(g *truetype.GlyphBuf, n int) (SDF2, bool) {
	// get the start and end point
	start := 0
	if n != 0 {
		start = g.Ends[n-1]
	}
	end := g.Ends[n] - 1

	// build a bezier curve from the points
	// work out the cw/ccw direction
	b := NewBezier()
	sum := 0.0
	off_prev := false
	v_prev := p_to_V2(g.Points[end])

	for i := start; i <= end; i++ {
		p := g.Points[i]
		v := p_to_V2(p)
		// is the point off/on the curve?
		off := p.Flags&1 == 0
		// do we have an implicit on point?
		if off && off_prev {
			// implicit on point at the midpoint of the 2 off points
			b.AddV2(v.Add(v_prev).MulScalar(0.5))
		}
		// add the point
		x := b.AddV2(v)
		if off {
			x.Mid()
		}
		// accumulate the cw/ccw direction
		sum += (v.X - v_prev.X) * (v.Y + v_prev.Y)
		// next point...
		v_prev = v
		off_prev = off
	}
	b.Close()

	return Polygon2D(b.Polygon().Vertices()), sum > 0
}

// return the SDF2 for a glyph
func glyph_convert(g *truetype.GlyphBuf) SDF2 {
	var s0 SDF2
	for n := 0; n < len(g.Ends); n++ {
		s1, cw := glyph_curve(g, n)
		if cw {
			s0 = Union2D(s0, s1)
		} else {
			s0 = Difference2D(s0, s1)
		}
	}
	return s0
}

//-----------------------------------------------------------------------------

// Return an SDF2 slice for a line of text
func lineSDF2(f *truetype.Font, l string) ([]SDF2, float64, error) {
	i_prev := truetype.Index(0)
	scale := fixed.Int26_6(f.FUnitsPerEm())
	x_ofs := 0.0

	var ss []SDF2

	for _, r := range l {
		i := f.Index(r)

		// get the glyph metrics
		hm := f.HMetric(scale, i)

		// apply kerning
		k := f.Kern(scale, i_prev, i)
		x_ofs += float64(k)
		i_prev = i

		// load the glyph
		g := &truetype.GlyphBuf{}
		err := g.Load(f, scale, i, font.HintingNone)
		if err != nil {
			return nil, 0, err
		}

		s := glyph_convert(g)
		if s != nil {
			s = Transform2D(s, Translate2d(V2{x_ofs, 0}))
			ss = append(ss, s)
		}

		x_ofs += float64(hm.AdvanceWidth)
	}

	return ss, x_ofs, nil
}

//-----------------------------------------------------------------------------
// public api

// NewText returns a text object (text and alignment).
func NewText(s string) *Text {
	return &Text{
		s:      s,
		halign: C_ALIGN,
	}
}

// LoadFont loads a truetype (*.ttf) font file.
func LoadFont(fname string) (*truetype.Font, error) {
	// read the font file
	b, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return truetype.Parse(b)
}

// TextSDF2 returns a sized SDF2 for a text object.
func TextSDF2(f *truetype.Font, t *Text, h float64) (SDF2, error) {
	scale := fixed.Int26_6(f.FUnitsPerEm())
	lines := strings.Split(t.s, "\n")
	y_ofs := 0.0
	vm := f.VMetric(scale, f.Index('\n'))
	ah := float64(vm.AdvanceHeight)

	var ss []SDF2

	for i := range lines {
		ss_line, hlen, err := lineSDF2(f, lines[i])
		if err != nil {
			return nil, err
		}
		x_ofs := 0.0
		if t.halign == R_ALIGN {
			x_ofs = -hlen
		} else if t.halign == C_ALIGN {
			x_ofs = -hlen / 2.0
		}
		for i := range ss_line {
			ss_line[i] = Transform2D(ss_line[i], Translate2d(V2{x_ofs, y_ofs}))
		}
		ss = append(ss, ss_line...)
		y_ofs -= ah
	}

	return CenterAndScale2D(Union2D(ss...), h/ah), nil
}

//-----------------------------------------------------------------------------
