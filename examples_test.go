package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// weeklyResetAt returns a BudgetResetAt string for a weekly budget where
// the given fraction of the period has already elapsed.
func weeklyResetAt(elapsedFraction float64) string {
	period := 7 * 24 * time.Hour
	remaining := time.Duration(float64(period) * (1 - elapsedFraction))
	return time.Now().UTC().Add(remaining).Format(time.RFC3339)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var ansiToSVGColor = map[string]string{
	ColorGreen:  "#22c55e",
	ColorYellow: "#eab308",
	ColorRed:    "#ef4444",
	ColorGray:   "#6b7280",
	ColorReset:  "#d1d5db",
}

func ansiToSpans(s string) string {
	var buf strings.Builder
	color := ansiToSVGColor[ColorReset]
	parts := ansiRe.Split(s, -1)
	codes := ansiRe.FindAllString(s, -1)

	for i, part := range parts {
		if part != "" {
			escaped := strings.ReplaceAll(strings.ReplaceAll(part, "&", "&amp;"), "<", "&lt;")
			buf.WriteString(fmt.Sprintf(`<tspan fill="%s">%s</tspan>`, color, escaped))
		}
		if i < len(codes) {
			if c, ok := ansiToSVGColor[codes[i]]; ok {
				color = c
			}
		}
	}
	return buf.String()
}

func TestGenerateExamples(t *testing.T) {
	t.Setenv("LITELLM_PLUGIN_BETA_FEATURES", "1")
	origVersion := Version
	defer func() { Version = origVersion }()
	Version = "v99.0.0"

	weekly := "7d"
	budget := 50.0

	type cell struct {
		percent         float64
		elapsedFraction float64
		possible        bool
	}

	// 3x3 grid: rows = absolute spend (fill color), columns = projected pace (marker color)
	grid := [3][3]cell{
		// Green fill (40% spent)
		{
			{40, 0.70, true},  // projected 57%  → green marker
			{40, 0.50, true},  // projected 80%  → yellow marker
			{40, 0.25, true},  // projected 160% → red marker
		},
		// Yellow fill (82% spent)
		{
			{0, 0, false},     // impossible: can't project < 75% with 82% spent
			{82, 0.95, true},  // projected 86%  → yellow marker
			{82, 0.55, true},  // projected 149% → red marker
		},
		// Red fill (95% spent)
		{
			{0, 0, false},     // impossible: can't project < 75% with 95% spent
			{95, 0.98, true},  // projected 97%  → yellow marker
			{95, 0.70, true},  // projected 136% → red marker
		},
	}

	rowLabels := []string{"&lt; 75% spent", "75-90% spent", "&gt; 90% spent"}
	colLabels := []string{"Under pace", "At pace", "Over pace"}

	// Layout
	padX, padY := 16, 16
	rowLabelWidth := 130
	colWidth := 200
	headerHeight := 28
	rowHeight := 36
	gridWidth := padX + rowLabelWidth + 3*colWidth + padX
	gridHeight := headerHeight + 3*rowHeight

	// Full status line section below grid
	sectionGap := 20
	statusLabelHeight := 18
	statusLineHeight := 18
	statusRowHeight := statusLabelHeight + statusLineHeight + 8
	statusCount := 3
	statusHeight := statusCount * statusRowHeight

	width := gridWidth
	height := padY + gridHeight + sectionGap + statusHeight + padY

	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" font-family="monospace" font-size="14">`, width, height))
	svg.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" rx="8" fill="#1e1e2e"/>`, width, height))

	// --- 3x3 Grid ---
	for j, label := range colLabels {
		x := padX + rowLabelWidth + j*colWidth + colWidth/2
		svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#9ca3af" text-anchor="middle" font-size="12">%s</text>`,
			x, padY+18, label))
	}
	svg.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#374151" stroke-width="1"/>`,
		padX, padY+headerHeight, width-padX, padY+headerHeight))

	for i := 0; i < 3; i++ {
		baseY := padY + headerHeight + i*rowHeight
		textY := baseY + rowHeight/2 + 5

		if i > 0 {
			svg.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#374151" stroke-width="1"/>`,
				padX, baseY, width-padX, baseY))
		}

		svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#9ca3af" font-size="12">%s</text>`,
			padX, textY, rowLabels[i]))

		for j := 0; j < 3; j++ {
			c := grid[i][j]
			cellX := padX + rowLabelWidth + j*colWidth

			svg.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#374151" stroke-width="1"/>`,
				cellX, padY+headerHeight, cellX, padY+gridHeight))

			if !c.possible {
				svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#4b5563" text-anchor="middle">—</text>`,
					cellX+colWidth/2, textY))
			} else {
				bar := renderProgressBar(c.percent, c.elapsedFraction, true)
				svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d">%s</text>`,
					cellX+8, textY, ansiToSpans(bar)))
			}
		}
	}

	// --- Full status line examples ---
	statusY := padY + gridHeight + sectionGap

	svg.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#374151" stroke-width="1"/>`,
		padX, statusY, width-padX, statusY))

	type statusExample struct {
		label string
		info  *KeyInfo
	}

	spend20 := 20.0
	spend30 := 30.0
	resetAt := weeklyResetAt(0.5)
	examples := []statusExample{
		{
			"Reset date available",
			&KeyInfo{Spend: &spend20, MaxBudget: &budget, BudgetResetAt: &resetAt, BudgetDuration: &weekly},
		},
		{
			"No reset date available",
			&KeyInfo{Spend: &spend30, MaxBudget: &budget},
		},
		{
			"No budget configured",
			&KeyInfo{Spend: &spend30},
		},
	}

	for i, ex := range examples {
		baseY := statusY + i*statusRowHeight

		if i > 0 {
			svg.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#374151" stroke-width="1"/>`,
				padX, baseY, width-padX, baseY))
		}

		svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#9ca3af" font-size="12">%s</text>`,
			padX, baseY+statusLabelHeight, ex.label))

		line := formatStatusLine(ex.info, "")
		svg.WriteString(fmt.Sprintf(`<text x="%d" y="%d">%s</text>`,
			padX, baseY+statusLabelHeight+statusLineHeight, ansiToSpans(line)))
	}

	svg.WriteString(`</svg>`)

	if err := os.WriteFile("examples.svg", []byte(svg.String()), 0644); err != nil {
		t.Fatal(err)
	}
	fmt.Println("wrote examples.svg")
}
