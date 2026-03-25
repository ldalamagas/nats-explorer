package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("#7B61FF")
	colorSecondary = lipgloss.Color("#6BCB77")
	colorMuted     = lipgloss.Color("#626262")
	colorError     = lipgloss.Color("#FF6B6B")
	colorBorder    = lipgloss.Color("#3C3C3C")
	colorSelected  = lipgloss.Color("#7B61FF")
	colorText      = lipgloss.Color("#DDDDDD")
	colorHeader    = lipgloss.Color("#FFFFFF")

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	StyleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	StylePrimary = lipgloss.NewStyle().
			Foreground(colorPrimary)

	StyleSecondary = lipgloss.NewStyle().
			Foreground(colorSecondary)

	StyleError = lipgloss.NewStyle().
			Foreground(colorError)

	StyleSelected = lipgloss.NewStyle().
			Foreground(colorSelected).
			Bold(true)

	StyleBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	StyleActiveBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary)

	StyleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Underline(true)

	StyleTabInactive = lipgloss.NewStyle().
				Foreground(colorMuted)

	StyleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleKeyHint = lipgloss.NewStyle().
			Foreground(colorMuted)

	StyleKeyName = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)
)
