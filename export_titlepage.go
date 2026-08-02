package main

import (
	"strconv"
	"strings"
)

// manuscriptWordCount sums the prose words across every block in the doc — the figure a
// manuscript title page reports. Approximate by construction (markup and endnotes included),
// which is why the title page rounds it.
func manuscriptWordCount(doc ManuscriptDoc) int {
	total := 0
	for _, sec := range doc {
		for _, blk := range sec.Blocks {
			total += blockWordCount(blk)
		}
	}
	return total
}

func blockWordCount(blk Block) int {
	switch v := blk.(type) {
	case Paragraph:
		return len(strings.Fields(plainText(v.Runs)))
	case Heading:
		return len(strings.Fields(plainText(v.Runs)))
	case Blockquote:
		n := 0
		for _, c := range v.Children {
			n += blockWordCount(c)
		}
		return n
	case List:
		n := 0
		for _, it := range v.Items {
			n += len(strings.Fields(plainText(it.Runs)))
		}
		return n
	case Endnotes:
		n := 0
		for _, e := range v.Items {
			n += len(strings.Fields(plainText(e.Runs)))
		}
		return n
	}
	return 0
}

// approxWords formats a rounded, comma-grouped word count for the title page, e.g.
// "~82,500 mots". Rounds to the nearest 100 under 1,000 words, else the nearest 500 —
// the "approximate" figure agents expect, never a false-precision exact count.
func approxWords(n int) string {
	step := 500
	if n < 1000 {
		step = 100
	}
	r := ((n + step/2) / step) * step
	return "~" + commaInt(r) + " mots"
}

// commaInt renders a non-negative int with thousands separators (1234567 → "1,234,567").
func commaInt(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// contactLines splits the OKASHI_CONTACT block into non-empty trimmed lines.
func contactLines(contact string) []string {
	var out []string
	for _, ln := range strings.Split(contact, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			out = append(out, t)
		}
	}
	return out
}
