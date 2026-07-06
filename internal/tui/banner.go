package tui

import "github.com/charmbracelet/lipgloss"

// bannerHeight is the number of terminal rows banner() renders, so the list
// below it can be sized correctly.
const bannerHeight = 8

// bannerArt is the CHARON wordmark (ANSI Shadow figlet style).
const bannerArt = ` ██████╗██╗  ██╗ █████╗ ██████╗  ██████╗ ███╗   ██╗
██╔════╝██║  ██║██╔══██╗██╔══██╗██╔═══██╗████╗  ██║
██║     ███████║███████║██████╔╝██║   ██║██╔██╗ ██║
██║     ██╔══██║██╔══██║██╔══██╗██║   ██║██║╚██╗██║
╚██████╗██║  ██║██║  ██║██║  ██║╚██████╔╝██║ ╚████║
 ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝`

var (
	bannerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	taglineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true).PaddingLeft(1)
)

// banner returns the styled splash shown atop the tool-selection screen.
func banner() string {
	return bannerStyle.Render(bannerArt) + "\n" +
		taglineStyle.Render("⛴  ferry your AI tools between endpoints · q to quit")
}
