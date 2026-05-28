package dashboard

import (
	"github.com/charmbracelet/lipgloss"

	"sshdash/internal/checks"
)

var (
	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("57")).
			Bold(true).
			Padding(0, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("159"))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("213")).
			Bold(true)

	nameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231"))

	summaryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("195"))

	detailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110")).
			Italic(true)

	latencyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("141"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("103"))

	weatherStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("31")).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	countStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1)

	summaryBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236")).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("146")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)
)

func panelStyle(title string) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(panelAccent(title)).
		Padding(1, 2).
		MarginTop(1)
}

func panelTitleStyle(title string) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("231")).
		Background(panelAccent(title)).
		Padding(0, 1).
		MarginBottom(1)
}

func panelAccent(title string) lipgloss.Color {
	switch title {
	case "Services":
		return lipgloss.Color("39")
	case "Docker Containers":
		return lipgloss.Color("45")
	case "Proxmox Health", "Proxmox VMs":
		return lipgloss.Color("208")
	case "PBS Health", "PBS Datastore Details":
		return lipgloss.Color("82")
	default:
		return lipgloss.Color("201")
	}
}

func statusDotStyle(status checks.Status) lipgloss.Style {
	style := lipgloss.NewStyle().
		Bold(true)

	switch status {
	case checks.StatusOK:
		return style.Foreground(lipgloss.Color("46"))
	case checks.StatusWarning:
		return style.Foreground(lipgloss.Color("220"))
	default:
		return style.Foreground(lipgloss.Color("196"))
	}
}

func statusTextStyle(status checks.Status) lipgloss.Style {
	style := lipgloss.NewStyle().
		Bold(true).
		Align(lipgloss.Center)

	switch status {
	case checks.StatusOK:
		return style.Foreground(lipgloss.Color("16")).Background(lipgloss.Color("82"))
	case checks.StatusWarning:
		return style.Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220"))
	default:
		return style.Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196"))
	}
}

func summaryBadgeStyle(status checks.Status) lipgloss.Style {
	return statusTextStyle(status)
}
