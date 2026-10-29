package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func fgbg(f, b color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(f).Background(b)
}

// lineBuf assembles one terminal line out of styled segments while keeping
// track of its printable width.
type lineBuf struct {
	sb strings.Builder
	w  int
}

func (l *lineBuf) add(s lipgloss.Style, text string) *lineBuf {
	if text == "" {
		return l
	}
	l.sb.WriteString(s.Render(text))
	l.w += ansi.StringWidth(text)
	return l
}

// raw appends an already rendered string of known printable width.
func (l *lineBuf) raw(text string, w int) *lineBuf {
	l.sb.WriteString(text)
	l.w += w
	return l
}

// padTo fills the line with styled spaces up to width w.
func (l *lineBuf) padTo(s lipgloss.Style, w int) *lineBuf {
	if w > l.w {
		l.add(s, strings.Repeat(" ", w-l.w))
	}
	return l
}

func (l *lineBuf) String() string { return l.sb.String() }

// truncate shortens plain text to at most w cells, ending in an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// fit pads or truncates an (optionally styled) string to exactly w cells.
func fit(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw > w {
		return ansi.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-sw)
}

// center centers plain text in w cells.
func center(s string, w int) string {
	s = truncate(s, w)
	left := (w - ansi.StringWidth(s)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-left-ansi.StringWidth(s))
}

// panel describes a rounded, titled box.
type panel struct {
	width, height int
	title, right  string // pre-rendered title segments for the top border
	footL, footR  string // pre-rendered segments for the bottom border
	focused       bool
	// optional scroll indicator drawn on the right border
	total, offset, visible int
}

func (m *Model) renderPanel(p panel, body []string) string {
	t := m.theme
	bc := t.Border
	if p.focused {
		bc = t.BorderFocus
	}
	bs := fg(bc)
	inner := p.width - 2

	border := func(left, right, l, r string) string {
		var lb lineBuf
		lb.add(bs, left)
		used := 0
		if l != "" {
			lb.add(bs, "─").raw(l, ansi.StringWidth(l))
			used += 1 + ansi.StringWidth(l)
		}
		rw := 0
		if r != "" {
			rw = ansi.StringWidth(r) + 1
		}
		if fill := inner - used - rw; fill > 0 {
			lb.add(bs, strings.Repeat("─", fill))
		}
		if r != "" {
			lb.raw(r, rw-1).add(bs, "─")
		}
		lb.add(bs, right)
		return lb.String()
	}

	rows := p.height - 2
	thumbStart, thumbEnd := -1, -1
	if p.total > p.visible && p.visible > 0 && rows > 0 {
		size := max(1, rows*p.visible/p.total)
		maxOff := p.total - p.visible
		thumbStart = (rows - size) * p.offset / max(1, maxOff)
		thumbEnd = thumbStart + size
	}

	out := make([]string, 0, p.height)
	out = append(out, border("╭", "╮", p.title, p.right))
	thumb := fg(t.Accent)
	for i := 0; i < rows; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		rb := bs.Render("│")
		if i >= thumbStart && i < thumbEnd {
			rb = thumb.Render("┃")
		}
		out = append(out, bs.Render("│")+fit(line, inner)+rb)
	}
	out = append(out, border("╰", "╯", p.footL, p.footR))
	return strings.Join(out, "\n")
}

// gradient returns n colors blended through the given stops.
func gradient(n int, stops ...color.Color) []color.Color {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []color.Color{stops[0]}
	}
	return lipgloss.Blend1D(n, stops...)
}

// ramp returns n colors spread evenly over the stops. Unlike gradient it
// always runs from the first stop to the last, however few colors are
// asked for, so a short rainbow still has both ends.
func ramp(n int, stops ...color.Color) []color.Color {
	if n <= 0 || len(stops) < 2 {
		return gradient(n, stops...)
	}
	cs := make([]colorful.Color, len(stops))
	for i, s := range stops {
		cs[i], _ = colorful.MakeColor(s)
	}
	out := make([]color.Color, n)
	for i := range out {
		pos := 0.0
		if n > 1 {
			pos = float64(i) / float64(n-1) * float64(len(cs)-1)
		}
		k := min(int(pos), len(cs)-2)
		out[i] = cs[k].BlendLab(cs[k+1], pos-float64(k)).Clamped()
	}
	return out
}
