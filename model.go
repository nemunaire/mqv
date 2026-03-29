package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// ── App state ─────────────────────────────────────────────────────────────────

type appState int

const (
	stateList    appState = iota
	stateMessage appState = iota
)

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	state           appState
	list            list.Model
	progress        progress.Model
	viewport        viewport.Model
	entries         []QueueEntry
	entryIndex      map[string]int // queue ID → entries index
	loadingTotal    int
	loadingDone     int
	currentRaw      string
	currentID       string
	showFullHeaders bool
	err             error
	saveNotice      string
	width           int
	height          int
}

func initialModel() Model {
	l := list.New(nil, itemDelegate{}, 80, 20)
	l.Title = "Postfix Queue"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.SetShowHelp(false)

	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	vp := viewport.New(80, 20)

	return Model{
		state:    stateList,
		list:     l,
		progress: prog,
		viewport: vp,
	}
}

// refreshViewport re-renders the current message and sets wrapped content.
func (m *Model) refreshViewport() {
	rendered := renderMessage(m.currentRaw, m.showFullHeaders)
	headerSection, body, _ := strings.Cut(rendered, "\n\n")

	var result []string
	for line := range strings.SplitSeq(headerSection, "\n") {
		parts := strings.Split(ansi.Wrap(line, m.viewport.Width-4, ""), "\n")
		result = append(result, parts[0])
		for _, continuation := range parts[1:] {
			result = append(result, "    "+continuation)
		}
	}
	result = append(result, "")
	result = append(result, strings.Split(ansi.Wrap(body, m.viewport.Width, ""), "\n")...)
	m.viewport.SetContent(strings.Join(result, "\n"))
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return loadQueueCmd()
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4
		m.list.SetSize(msg.Width, msg.Height-2)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 2
		if m.state == stateMessage && m.currentRaw != "" {
			m.refreshViewport()
		}
		return m, nil

	case queueParsedMsg:
		m.entries = msg.entries
		m.entryIndex = make(map[string]int, len(msg.entries))
		m.loadingTotal = len(msg.entries)
		m.loadingDone = 0
		m.err = nil

		items := make([]list.Item, len(msg.entries))
		for i, e := range msg.entries {
			items[i] = queueItem{e}
			m.entryIndex[e.ID] = i
		}
		m.list.SetItems(items)

		if m.loadingTotal == 0 {
			return m, nil
		}
		cmds := make([]tea.Cmd, len(msg.entries))
		for i, e := range msg.entries {
			cmds[i] = fetchSubjectCmd(e.ID)
		}
		return m, tea.Batch(cmds...)

	case subjectFetchedMsg:
		if idx, ok := m.entryIndex[msg.id]; ok {
			m.entries[idx].Subject = msg.subject
			m.list.SetItem(idx, queueItem{m.entries[idx]})
		}
		m.loadingDone++
		pct := float64(m.loadingDone) / float64(m.loadingTotal)
		return m, m.progress.SetPercent(pct)

	case progress.FrameMsg:
		pm, cmd := m.progress.Update(msg)
		m.progress = pm.(progress.Model)
		return m, cmd

	case queueErrMsg:
		m.err = msg.err
		return m, nil

	case messageLoadedMsg:
		m.currentRaw = msg.content
		m.currentID = msg.id
		m.showFullHeaders = false
		m.refreshViewport()
		m.viewport.GotoTop()
		m.state = stateMessage
		return m, nil

	case messageErrMsg:
		m.err = msg.err
		return m, nil

	case savedMsg:
		m.saveNotice = "Saved: " + msg.path
		return m, nil

	case saveErrMsg:
		m.saveNotice = "Save error: " + msg.err.Error()
		return m, nil

	case tea.KeyMsg:
		switch m.state {

		case stateList:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "r":
				m.entries = nil
				m.entryIndex = nil
				m.loadingTotal = 0
				m.loadingDone = 0
				m.list.SetItems(nil)
				_ = m.progress.SetPercent(0)
				return m, loadQueueCmd()
			case "enter":
				if item, ok := m.list.SelectedItem().(queueItem); ok {
					return m, loadMessageCmd(item.entry.ID)
				}
			}

		case stateMessage:
			switch msg.String() {
			case "q":
				m.state = stateList
				m.saveNotice = ""
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "s":
				return m, saveCmd(m.currentID, m.currentRaw)
			case "H":
				m.showFullHeaders = !m.showFullHeaders
				m.refreshViewport()
				m.viewport.GotoTop()
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	switch m.state {
	case stateList:
		m.list, cmd = m.list.Update(msg)
	case stateMessage:
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.err != nil {
		return errorStyle.Render("Error: "+m.err.Error()) + "\n\nPress q to quit."
	}

	switch m.state {

	case stateList:
		if m.entries == nil {
			return titleStyle.Render("Postfix Queue") + "\n\n" +
				dimStyle.Render("  Fetching queue list…")
		}
		return m.list.View() + "\n" + m.renderBottom()

	case stateMessage:
		header := titleStyle.Render(fmt.Sprintf("Message: %s", m.currentID))
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		headersHint := "H: full headers"
		if m.showFullHeaders {
			headersHint = "H: short headers"
		}
		status := statusBarStyle.Render(
			fmt.Sprintf(" ↑↓/SPC/PgUp/Dn: scroll │ s: save EML │ %s │ q: back │ %d%% ", headersHint, scrollPct),
		)
		notice := ""
		if m.saveNotice != "" {
			notice = "\n" + saveNoticeStyle.Render(m.saveNotice)
		}
		return header + "\n" + m.viewport.View() + "\n" + status + notice
	}

	return ""
}

func (m Model) renderBottom() string {
	if m.loadingDone < m.loadingTotal {
		label := fmt.Sprintf("  Fetching subjects: %d / %d  ", m.loadingDone, m.loadingTotal)
		return dimStyle.Render(label) + "\n  " + m.progress.View()
	}
	return statusBarStyle.Render(
		fmt.Sprintf(" %d message(s) │ Enter: open │ r: refresh │ q: quit ", len(m.list.Items())),
	)
}
