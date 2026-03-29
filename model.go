package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// ── App state ─────────────────────────────────────────────────────────────────

type appState int

const (
	stateList     appState = iota
	stateMessage  appState = iota
	stateParts    appState = iota
	statePartView appState = iota
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
	messageSaving   bool
	// stateParts fields
	parts          []MessagePart
	partsCursor    int
	partsSaving    bool
	partsSaveInput textinput.Model
	partSaveNotice string
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

	ti := textinput.New()
	ti.Placeholder = "filename"
	ti.CharLimit = 255

	return Model{
		state:          stateList,
		list:           l,
		progress:       prog,
		viewport:       vp,
		partsSaveInput: ti,
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
		} else if m.state == statePartView && len(m.parts) > 0 {
			p := m.parts[m.partsCursor]
			content := ansi.Wrap(renderPart(p.CT, "", "", bytes.NewReader(p.Data)), m.viewport.Width, "")
			m.viewport.SetContent(content)
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

	case partSavedMsg:
		m.partSaveNotice = "Saved: " + msg.path
		return m, nil

	case partSaveErrMsg:
		m.partSaveNotice = "Save error: " + msg.err.Error()
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
			case "F":
				m.entries = nil
				m.entryIndex = nil
				m.loadingTotal = 0
				m.loadingDone = 0
				m.list.SetItems(nil)
				_ = m.progress.SetPercent(0)
				return m, flushQueueCmd()
			case "enter":
				if item, ok := m.list.SelectedItem().(queueItem); ok {
					return m, loadMessageCmd(item.entry.ID)
				}
			}

		case stateMessage:
			if m.messageSaving {
				switch msg.String() {
				case "esc":
					m.messageSaving = false
					m.partsSaveInput.Blur()
					return m, nil
				case "enter":
					name := strings.TrimSpace(m.partsSaveInput.Value())
					content := m.currentRaw
					m.messageSaving = false
					m.partsSaveInput.Blur()
					return m, saveMessageAsCmd(content, name)
				default:
					var cmd tea.Cmd
					m.partsSaveInput, cmd = m.partsSaveInput.Update(msg)
					return m, cmd
				}
			}
			switch msg.String() {
			case "q":
				m.state = stateList
				m.saveNotice = ""
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "s":
				m.messageSaving = true
				m.saveNotice = ""
				m.partsSaveInput.SetValue(m.currentID + ".eml")
				m.partsSaveInput.CursorEnd()
				m.partsSaveInput.Focus()
				return m, textinput.Blink
			case "H":
				m.showFullHeaders = !m.showFullHeaders
				m.refreshViewport()
				m.viewport.GotoTop()
				return m, nil
			case "h":
				if idx, ok := m.entryIndex[m.currentID]; ok {
					if m.entries[idx].OnHold {
						m.entries[idx].OnHold = false
						return m, releaseMessageCmd(m.currentID)
					}
					m.entries[idx].OnHold = true
					return m, holdMessageCmd(m.currentID)
				}
			case "D":
				id := m.currentID
				m.state = stateList
				m.saveNotice = ""
				return m, deleteMessageCmd(id)
			case "F":
				return m, requeueMessageCmd(m.currentID)
			case "v":
				m.parts = extractParts(m.currentRaw)
				m.partsCursor = 0
				m.partsSaving = false
				m.partSaveNotice = ""
				m.partsSaveInput.SetValue("")
				m.partsSaveInput.Blur()
				m.state = stateParts
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case stateParts:
			if m.partsSaving {
				switch msg.String() {
				case "esc":
					m.partsSaving = false
					m.partsSaveInput.Blur()
					return m, nil
				case "enter":
					name := strings.TrimSpace(m.partsSaveInput.Value())
					part := m.parts[m.partsCursor]
					m.partsSaving = false
					m.partsSaveInput.Blur()
					return m, savePartCmd(part.Data, name)
				default:
					var cmd tea.Cmd
					m.partsSaveInput, cmd = m.partsSaveInput.Update(msg)
					return m, cmd
				}
			}
			switch msg.String() {
			case "q", "esc":
				m.state = stateMessage
				m.partSaveNotice = ""
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				if m.partsCursor > 0 {
					m.partsCursor--
				}
				return m, nil
			case "down", "j":
				if m.partsCursor < len(m.parts)-1 {
					m.partsCursor++
				}
				return m, nil
			case "enter":
				if len(m.parts) > 0 {
					p := m.parts[m.partsCursor]
					if strings.HasPrefix(p.MediaType, "text/") {
						content := ansi.Wrap(renderPart(p.CT, "", "", bytes.NewReader(p.Data)), m.viewport.Width, "")
						m.viewport.SetContent(content)
						m.viewport.GotoTop()
						m.state = statePartView
						return m, nil
					}
				}
			case "s":
				if len(m.parts) > 0 {
					m.partsSaving = true
					m.partSaveNotice = ""
					m.partsSaveInput.SetValue(m.parts[m.partsCursor].Name)
					m.partsSaveInput.CursorEnd()
					m.partsSaveInput.Focus()
					return m, textinput.Blink
				}
			}

		case statePartView:
			switch msg.String() {
			case "q", "esc":
				m.state = stateParts
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
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
		title := titleStyle.Render(fmt.Sprintf("Message: %s", m.currentID))
		header := title
		if idx, ok := m.entryIndex[m.currentID]; ok {
			if reason := m.entries[idx].Reason; reason != "" {
				gap := m.width - lipgloss.Width(title) - lipgloss.Width(reason) - 2
				if gap < 1 {
					gap = 1
				}
				header = title + strings.Repeat(" ", gap) + reasonStyle.Render(reason)
			}
		}
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		headersHint := "H: full headers"
		if m.showFullHeaders {
			headersHint = "H: short headers"
		}
		holdHint := "h: hold"
		if idx, ok := m.entryIndex[m.currentID]; ok && m.entries[idx].OnHold {
			holdHint = "h: release"
		}
		status := statusBarStyle.Render(
			fmt.Sprintf(" ↑↓/SPC/PgUp/Dn: scroll │ s: save EML │ D: delete │ F: requeue │ %s │ v: parts │ %s │ q: back │ %d%% ", holdHint, headersHint, scrollPct),
		)
		notice := ""
		if m.messageSaving {
			notice = "\n Save as: " + m.partsSaveInput.View()
		} else if m.saveNotice != "" {
			notice = "\n" + saveNoticeStyle.Render(m.saveNotice)
		}
		return header + "\n" + m.viewport.View() + "\n" + status + notice

	case stateParts:
		title := titleStyle.Render(fmt.Sprintf("Parts: %s", m.currentID))

		// Compute name column width from remaining terminal space.
		// Columns: #(3) + gap(2) + CT(30) + gap(2) + size(10) + gap(2) = 49
		nameW := m.width - 49
		if nameW < 8 {
			nameW = 8
		}

		header := dimStyle.Render(fmt.Sprintf(
			"%-3s  %-30s  %-*s  %10s",
			"#", "Content-Type", nameW, "Name", "Size",
		))

		var rows strings.Builder
		for i, p := range m.parts {
			name := p.Name
			if name == "" {
				name = "-"
			}
			if len(name) > nameW {
				name = name[:nameW-1] + "…"
			}
			line := fmt.Sprintf(
				"%-3d  %-30s  %-*s  %10s",
				p.Index, p.MediaType, nameW, name, formatSize(p.Size),
			)
			if i == m.partsCursor {
				rows.WriteString(selectedStyle.Render(line))
			} else {
				rows.WriteString(line)
			}
			rows.WriteByte('\n')
		}
		if len(m.parts) == 0 {
			rows.WriteString(dimStyle.Render("  (no parts found)"))
			rows.WriteByte('\n')
		}

		status := statusBarStyle.Render(" ↑↓/j/k: navigate │ enter: view text part │ s: save part │ q/esc: back ")

		bottom := ""
		if m.partsSaving {
			bottom = "\n Save as: " + m.partsSaveInput.View()
		} else if m.partSaveNotice != "" {
			bottom = "\n" + saveNoticeStyle.Render(m.partSaveNotice)
		}

		return title + "\n" + header + "\n" + rows.String() + status + bottom

	case statePartView:
		p := m.parts[m.partsCursor]
		title := titleStyle.Render(fmt.Sprintf("Part %d: %s", p.Index, p.MediaType))
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		status := statusBarStyle.Render(
			fmt.Sprintf(" ↑↓/SPC/PgUp/Dn: scroll │ q/esc: back to parts │ %d%% ", scrollPct),
		)
		return title + "\n" + m.viewport.View() + "\n" + status
	}

	return ""
}

func (m Model) renderBottom() string {
	if m.loadingDone < m.loadingTotal {
		label := fmt.Sprintf("  Fetching subjects: %d / %d  ", m.loadingDone, m.loadingTotal)
		return dimStyle.Render(label) + "\n  " + m.progress.View()
	}
	left := fmt.Sprintf(" %d message(s) │ Enter: open │ r: refresh │ F: flush │ q: quit ", len(m.list.Items()))
	reason := ""
	if item, ok := m.list.SelectedItem().(queueItem); ok {
		reason = item.entry.Reason
	}
	if reason == "" {
		return statusBarStyle.Render(left)
	}
	right := " " + reason + " "
	rightWidth := lipgloss.Width(right)
	leftPart := statusBarStyle.Width(m.width - rightWidth).Render(left)
	rightPart := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Background(lipgloss.Color("241")).Render(right)
	return leftPart + rightPart
}
