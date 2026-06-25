package main

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)

	dayStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("240")).
			MarginTop(1)

	todayStyle = dayStyle.Foreground(lipgloss.Color("212"))

	timeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Width(13)

	allDayStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Width(13)

	summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	calStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

	locStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	emptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)

	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func render(w io.Writer, events []Event, start time.Time, days int) {
	header := "📅  Calendar"
	if days > 1 {
		header = fmt.Sprintf("%s — next %d days", header, days)
	}
	fmt.Fprintln(w, titleStyle.Render(header))

	today := start
	byDay := map[string][]Event{}
	for _, e := range events {
		key := e.Start.Format("2006-01-02")
		byDay[key] = append(byDay[key], e)
	}

	for d := range days {
		day := start.AddDate(0, 0, d)
		key := day.Format("2006-01-02")
		label := day.Format("Mon, Jan 2")
		if day.Equal(today) {
			label += "  ·  today"
			fmt.Fprintln(w, todayStyle.Render(label))
		} else if day.Equal(today.AddDate(0, 0, 1)) {
			label += "  ·  tomorrow"
			fmt.Fprintln(w, dayStyle.Render(label))
		} else {
			fmt.Fprintln(w, dayStyle.Render(label))
		}

		evs := byDay[key]
		if len(evs) == 0 {
			fmt.Fprintln(w, "  "+emptyStyle.Render("—"))
			continue
		}
		for _, e := range evs {
			var when string
			if e.AllDay {
				when = allDayStyle.Render("all day")
			} else {
				when = timeStyle.Render(e.Start.Format("15:04") + "–" + e.End.Format("15:04"))
			}
			line := "  " + when + summaryStyle.Render(e.Summary) + "  " + calStyle.Render("("+e.Cal+")")
			fmt.Fprintln(w, line)
			if e.Location != "" {
				fmt.Fprintln(w, "  "+lipgloss.NewStyle().Width(13).Render("")+locStyle.Render("@ "+e.Location))
			}
		}
	}
}
