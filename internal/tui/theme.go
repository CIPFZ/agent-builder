package tui

import "charm.land/lipgloss/v2"

// Theme colors inspired by Claude Code's dark theme
var (
	// Brand / Primary
	ClaudeOrange     = lipgloss.Color("#D4A373")
	ClaudeOrangeBold = lipgloss.Color("#E9C46A")

	// Semantic colors
	SuccessGreen  = lipgloss.Color("#52B788")
	ErrorRed      = lipgloss.Color("#EF476F")
	WarningYellow = lipgloss.Color("#FFD166")
	InfoBlue      = lipgloss.Color("#118AB2")

	// Permission / Tool colors
	PermissionBlue = lipgloss.Color("#07AAFF")
	ToolPurple     = lipgloss.Color("#9B5DE5")

	// Background colors
	DarkBg         = lipgloss.Color("#0D1117")
	DarkBgElevated = lipgloss.Color("#161B22")
	DarkBgSubtle   = lipgloss.Color("#21262D")
	DarkBorder     = lipgloss.Color("#30363D")

	// User message background (gray)
	UserMsgBg = lipgloss.Color("#5e5c64")

	// Text colors
	DarkText       = lipgloss.Color("#E6EDF3")
	DarkTextMuted  = lipgloss.Color("#8B949E")
	DarkTextSubtle = lipgloss.Color("#6E7681")

	// Role colors
	UserRoleColor  = lipgloss.Color("#58A6FF")
	AssistantColor = ClaudeOrange
	ToolColor      = ToolPurple
	SystemColor    = DarkTextMuted
)

// Claude Code style border definitions
var (
	// Top and bottom borders with title
	TopLeftCorner = lipgloss.Color("#AB4682") // Pink/magenta for Claude branding
	BorderColor   = DarkBorder

	// Panel border - rounded style
	RoundedBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "─",
		BottomLeft:  "╰",
		BottomRight: "─",
	}

	// Full rounded border
	FullRoundedBorder = lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
	}
)

// Text styles
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1F1F1F")).
			Bold(true)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(DarkTextMuted)

	LabelStyle = lipgloss.NewStyle().
			Foreground(DarkTextMuted)

	ValueStyle = lipgloss.NewStyle().
			Foreground(DarkText)

	DimStyle = lipgloss.NewStyle().
			Foreground(DarkTextSubtle)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ErrorRed)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(SuccessGreen)

	WarningStyle = lipgloss.NewStyle().
			Foreground(WarningYellow)
)

// Message styles
var (
	UserMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(UserMsgBg).
				Bold(true)

	AssistantMessageStyle = lipgloss.NewStyle().
				Foreground(AssistantColor)

	ToolMessageStyle = lipgloss.NewStyle().
				Foreground(ToolColor)

	SystemMessageStyle = lipgloss.NewStyle().
				Foreground(SystemColor)
)

// Panel styles
var (
	PanelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(DarkBorder).
			Padding(0, 1)

	PanelTitleStyle = lipgloss.NewStyle().
			Foreground(DarkTextMuted)
)

// Input styles
var (
	InputPromptStyle = lipgloss.NewStyle().
				Foreground(ClaudeOrangeBold).
				Bold(true)

	InputTextStyle = lipgloss.NewStyle().
			Foreground(DarkText)

	CursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#FFFFFF")).
			Foreground(DarkBg).
			Bold(true)
)

// Approval styles
var (
	ApprovalBoxStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(PermissionBlue).
				Foreground(lipgloss.Color("#1F1F1F")).
				Padding(1, 2)

	ApprovalTitleStyle = lipgloss.NewStyle().
				Foreground(PermissionBlue).
				Bold(true)

	ApprovalToolStyle = lipgloss.NewStyle().
				Foreground(ToolColor)

	ApprovalInputStyle = lipgloss.NewStyle().
				Foreground(DarkTextMuted)
)

// Status styles - these return the rendered string directly
var (
	StatusBusyStyle  = lipgloss.NewStyle().Foreground(ClaudeOrange).Render("●")
	StatusIdleStyle  = lipgloss.NewStyle().Foreground(DarkTextSubtle).Render("○")
	StatusErrorStyle = lipgloss.NewStyle().Foreground(ErrorRed).Render("✗")
)

// Spinner styles
var (
	SpinnerStyle = lipgloss.NewStyle().
		Foreground(ClaudeOrange)
)

// Border styles for sections
var (
	BorderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(DarkBorder)

	HighlightBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ClaudeOrange)
)
