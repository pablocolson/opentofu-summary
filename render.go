package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// RenderOptions controls how a Summary is printed.
type RenderOptions struct {
	Color    bool // emit ANSI color escapes
	Verbose  bool // include per-module resource-group breakdown
	NoHeader bool // skip the top banner (useful when embedded in other output)
}

// ANSI sequences. Kept inline rather than depending on a color library.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
)

func (o RenderOptions) color(seq, s string) string {
	if !o.Color {
		return s
	}
	return seq + s + ansiReset
}

// Render writes a human-readable summary to w.
func Render(w io.Writer, s *Summary, opts RenderOptions) error {
	if !opts.NoHeader {
		if err := renderHeader(w, s, opts); err != nil {
			return err
		}
	}
	// Empty plan — stop after the "no changes" header line. The downstream
	// sections would all be empty and the trailing "No replaces..." line is
	// noisy when there's literally nothing to report.
	if s.Totals.Total() == 0 {
		return nil
	}
	if err := renderByType(w, s, opts); err != nil {
		return err
	}
	if err := renderByModule(w, s, opts); err != nil {
		return err
	}
	if opts.Verbose {
		if err := renderModuleDetail(w, s, opts); err != nil {
			return err
		}
	}
	if err := renderWarnings(w, s, opts); err != nil {
		return err
	}
	return nil
}

func renderHeader(w io.Writer, s *Summary, opts RenderOptions) error {
	t := s.Totals
	if t.Total() == 0 {
		// No actionable changes (everything is a no-op or a data read).
		_, err := fmt.Fprintln(w, opts.color(ansiDim, "Plan: no changes."))
		return err
	}

	var parts []string
	if t.Create > 0 {
		parts = append(parts, opts.color(ansiGreen, fmt.Sprintf("+%d add", t.Create)))
	}
	if t.Update > 0 {
		parts = append(parts, opts.color(ansiYellow, fmt.Sprintf("~%d change", t.Update)))
	}
	if t.Delete > 0 {
		parts = append(parts, opts.color(ansiRed, fmt.Sprintf("-%d destroy", t.Delete)))
	}
	if t.Replace > 0 {
		parts = append(parts, opts.color(ansiRed, fmt.Sprintf("±%d replace", t.Replace)))
	}
	if t.Forget > 0 {
		parts = append(parts, opts.color(ansiDim, fmt.Sprintf("%d forget", t.Forget)))
	}

	title := opts.color(ansiBold, "Plan summary")
	_, err := fmt.Fprintf(w, "%s  %s\n", title, strings.Join(parts, "  "))
	return err
}

func renderByType(w io.Writer, s *Summary, opts RenderOptions) error {
	if len(s.ByType) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, opts.color(ansiBold, "By resource type"))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	for _, tb := range s.ByType {
		if tb.Counts.Total() == 0 {
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n",
			actionGlyphs(tb.Counts, opts),
			tb.Type,
			modulesPhrase(len(tb.Modules)),
		)
	}
	return nil
}

func renderByModule(w io.Writer, s *Summary, opts RenderOptions) error {
	if len(s.ByModule) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, opts.color(ansiBold, "By module"))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	defer tw.Flush()
	for _, mb := range s.ByModule {
		if mb.Counts.Total() == 0 {
			continue
		}
		name := mb.Module
		if name == "" {
			name = opts.color(ansiDim, "<root>")
		}
		fmt.Fprintf(tw, "  %s\t%s\n",
			name,
			actionGlyphs(mb.Counts, opts),
		)
	}
	return nil
}

func renderModuleDetail(w io.Writer, s *Summary, opts RenderOptions) error {
	if len(s.ByModule) == 0 {
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, opts.color(ansiBold, "Detail"))
	for _, mb := range s.ByModule {
		if len(mb.Groups) == 0 {
			continue
		}
		name := mb.Module
		if name == "" {
			name = "<root>"
		}
		fmt.Fprintf(w, "  %s  %s\n", name, actionGlyphs(mb.Counts, opts))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, g := range mb.Groups {
			label := g.Type + "." + g.Name
			fmt.Fprintf(tw, "    %s\t%d\t%s\n",
				actionGlyph(g.Action, opts),
				g.Count,
				label,
			)
		}
		tw.Flush()
	}
	return nil
}

func renderWarnings(w io.Writer, s *Summary, opts RenderOptions) error {
	if len(s.Warnings) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, opts.color(ansiDim, "No replaces, no destroys of persistent resources."))
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, opts.color(ansiBold, "Pay attention"))
	for _, ww := range s.Warnings {
		var glyph string
		switch ww.Severity {
		case "danger":
			glyph = opts.color(ansiRed, "!")
		case "warn":
			glyph = opts.color(ansiYellow, "!")
		default:
			glyph = opts.color(ansiCyan, "i")
		}
		fmt.Fprintf(w, "  %s %s\n", glyph, ww.Message)
	}
	return nil
}

// actionGlyphs renders a Counts as a compact line like "+24 -14".
func actionGlyphs(c Counts, opts RenderOptions) string {
	var parts []string
	if c.Create > 0 {
		parts = append(parts, opts.color(ansiGreen, fmt.Sprintf("+%d", c.Create)))
	}
	if c.Update > 0 {
		parts = append(parts, opts.color(ansiYellow, fmt.Sprintf("~%d", c.Update)))
	}
	if c.Delete > 0 {
		parts = append(parts, opts.color(ansiRed, fmt.Sprintf("-%d", c.Delete)))
	}
	if c.Replace > 0 {
		parts = append(parts, opts.color(ansiRed, fmt.Sprintf("±%d", c.Replace)))
	}
	if c.Forget > 0 {
		parts = append(parts, opts.color(ansiDim, fmt.Sprintf("%d forget", c.Forget)))
	}
	if len(parts) == 0 {
		return opts.color(ansiDim, "·")
	}
	return strings.Join(parts, " ")
}

func actionGlyph(a Action, opts RenderOptions) string {
	switch a {
	case ActionCreate:
		return opts.color(ansiGreen, "+")
	case ActionUpdate:
		return opts.color(ansiYellow, "~")
	case ActionDelete:
		return opts.color(ansiRed, "-")
	case ActionReplace:
		return opts.color(ansiRed, "±")
	case ActionForget:
		return opts.color(ansiDim, "x")
	case ActionRead:
		return opts.color(ansiCyan, "?")
	default:
		return " "
	}
}

func modulesPhrase(n int) string {
	if n == 1 {
		return "in 1 module"
	}
	return fmt.Sprintf("in %d modules", n)
}
