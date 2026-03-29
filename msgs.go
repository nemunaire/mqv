package main

import (
	"os/exec"

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
type partSavedMsg struct{ path string }
type partSaveErrMsg struct{ err error }

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

func flushQueueCmd() tea.Cmd {
	return func() tea.Msg {
		exec.Command("postqueue", "-f").Run()
		entries, err := loadQueue()
		if err != nil {
			return queueErrMsg{err}
		}
		return queueParsedMsg{entries}
	}
}

func holdMessageCmd(id string) tea.Cmd {
	return func() tea.Msg {
		exec.Command("postsuper", "-h", id).Run()
		return nil
	}
}

func releaseMessageCmd(id string) tea.Cmd {
	return func() tea.Msg {
		exec.Command("postsuper", "-H", id).Run()
		return nil
	}
}

func deleteMessageCmd(id string) tea.Cmd {
	return func() tea.Msg {
		exec.Command("postsuper", "-d", id).Run()
		return nil
	}
}

func requeueMessageCmd(id string) tea.Cmd {
	return func() tea.Msg {
		exec.Command("postsuper", "-r", id).Run()
		return nil
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

func saveMessageAsCmd(content, name string) tea.Cmd {
	return func() tea.Msg {
		path, err := savePart([]byte(content), name)
		if err != nil {
			return saveErrMsg{err}
		}
		return savedMsg{path}
	}
}

func savePartCmd(data []byte, name string) tea.Cmd {
	return func() tea.Msg {
		path, err := savePart(data, name)
		if err != nil {
			return partSaveErrMsg{err}
		}
		return partSavedMsg{path}
	}
}
