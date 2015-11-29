package latlong

import "github.com/golang/freetype/raster"

// A MonochromePainter wraps another Painter, quantizing each Span's alpha to
// be either fully opaque or fully transparent.
type MyMonochromePainter struct {
	Painter   raster.Painter
	y, x0, x1 int
}

// Paint delegates to the wrapped Painter after quantizing each Span's alpha
// value and merging adjacent fully opaque Spans.
func (m *MyMonochromePainter) Paint(ss []raster.Span, done bool) {
	// We compact the ss slice, discarding any Spans whose alpha quantizes to zero.
	j := 0
	for _, s := range ss {
		if s.Alpha >= 0x8000 {
			if m.y == s.Y && m.x1 == s.X0 {
				m.x1 = s.X1
			} else {
				ss[j] = raster.Span{m.y, m.x0, m.x1, 0xffff}
				j++
				m.y, m.x0, m.x1 = s.Y, s.X0, s.X1
			}
		}
	}
	if done {
		// Flush the accumulated Span.
		finalSpan := raster.Span{m.y, m.x0, m.x1, 0xffff}
		if j < len(ss) {
			ss[j] = finalSpan
			j++
			m.Painter.Paint(ss[:j], true)
		} else if j == len(ss) {
			m.Painter.Paint(ss, false)
			if cap(ss) > 0 {
				ss = ss[:1]
			} else {
				ss = make([]raster.Span, 1)
			}
			ss[0] = finalSpan
			m.Painter.Paint(ss, true)
		} else {
			panic("unreachable")
		}
		// Reset the accumulator, so that this Painter can be re-used.
		m.y, m.x0, m.x1 = 0, 0, 0
	} else {
		m.Painter.Paint(ss[:j], false)
	}
}