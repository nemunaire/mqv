package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ── Tea message types ─────────────────────────────────────────────────────────

type queueParsedMsg struct{ entries []QueueEntry } // phase 1: structure only
type subjectFetchedMsg struct {                    // phase 2: one per entry
	id      string
	subject string
}
type queueErrMsg struct{ err error }
type messageLoadedMsg struct {
	id      string
	content string
}
type messageErrMsg struct{ err error }
type savedMsg struct{ path string }
type saveErrMsg struct{ err error }

// ── Tea commands ──────────────────────────────────────────────────────────────

func loadQueueCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := loadQueue()
		if err != nil {
			return queueErrMsg{err}
		}
		return queueParsedMsg{entries}
	}
}

func fetchSubjectCmd(id string) tea.Cmd {
	return func() tea.Msg {
		raw, err := fetchHeaders(id)
		if err != nil {
			return subjectFetchedMsg{id: id, subject: "(error)"}
		}
		return subjectFetchedMsg{id: id, subject: extractSubject(raw)}
	}
}

func loadMessageCmd(id string) tea.Cmd {
	return func() tea.Msg {
		content, err := fetchMessage(id)
		if err != nil {
			return messageErrMsg{err}
		}
		return messageLoadedMsg{id: id, content: content}
	}
}

func saveCmd(id, content string) tea.Cmd {
	return func() tea.Msg {
		path, err := saveMessage(id, content)
		if err != nil {
			return saveErrMsg{err}
		}
		return savedMsg{path}
	}
}
