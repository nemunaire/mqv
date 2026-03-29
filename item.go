package main

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type queueItem struct {
	entry QueueEntry
}

func (q queueItem) FilterValue() string { return q.entry.Sender + " " + q.entry.Subject }
func (q queueItem) Title() string       { return q.entry.Subject }
func (q queueItem) Description() string {
	return fmt.Sprintf("%-40s  %s", q.entry.Sender, q.entry.Date.Format("Jan 02 15:04"))
}

// itemDelegate renders each queue entry as a single line.
type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	qi, ok := item.(queueItem)
	if !ok {
		return
	}
	e := qi.entry
	date := e.Date.Format("Jan _2 15:04")
	from := e.Sender
	if len(from) > 30 {
		from = from[:28] + ".."
	}
	subj := e.Subject
	if subj == "" {
		subj = dimStyle.Render("loading…")
	}

	line := fmt.Sprintf("  %-10s  %-14s  %-30s  %s", e.ID, date, from, subj)

	if index == m.Index() {
		fmt.Fprint(w, selectedStyle.Render(line))
	} else {
		fmt.Fprint(w, line)
	}
}
