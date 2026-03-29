package main

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusBarStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Reverse(true).Padding(0, 1)
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	saveNoticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	partSepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	attachStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	headerKeyStyle  = lipgloss.NewStyle().Bold(true)
	reasonStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)
