package dashboard

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/muesli/termenv"

	"sshdash/internal/checks"
	"sshdash/internal/config"
)

type model struct {
	checkers []checks.Checker
	results  []checks.Result
	refresh  time.Duration
	spinner  spinner.Model
	width    int
	height   int
	err      error
}

type checkResultsMsg []checks.Result

type errMsg struct {
	err error
}

func NewProgram(cfg config.Config) func(ssh.Session) (tea.Model, []tea.ProgramOption) {
	return func(_ ssh.Session) (tea.Model, []tea.ProgramOption) {
		lipgloss.SetColorProfile(termenv.ANSI256)

		spin := spinner.New()
		spin.Spinner = spinner.Dot
		spin.Style = spinnerStyle

		return model{
			checkers: checks.FromConfig(cfg),
			refresh:  cfg.Refresh,
			spinner:  spin,
		}, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.runChecks())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			return m, m.runChecks()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case checkResultsMsg:
		m.err = nil
		m.results = []checks.Result(msg)
		return m, tea.Tick(m.refresh, func(time.Time) tea.Msg {
			return tickMsg{}
		})
	case errMsg:
		m.err = msg.err
		return m, nil
	case tickMsg:
		return m, m.runChecks()
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	header := headerStyle.Width(max(42, min(m.width, 110))).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			titleStyle.Render("sshdash"),
			" ",
			subtitleStyle.Render("home network dashboard"),
		),
	)
	help := helpStyle.Render("r refresh  q quit")

	body := m.renderBody()
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", help)

	if m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
	}
	return content
}

type tickMsg struct{}

func (m model) runChecks() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.refresh)
		defer cancel()

		results := make([]checks.Result, 0, len(m.checkers))
		for _, checker := range m.checkers {
			results = append(results, checker.Check(ctx))
		}
		sort.Slice(results, func(i, j int) bool {
			if results[i].Kind == results[j].Kind {
				return results[i].Name < results[j].Name
			}
			return results[i].Kind < results[j].Kind
		})
		return checkResultsMsg(results)
	}
}

func (m model) renderBody() string {
	if m.err != nil {
		return errorStyle.Render(m.err.Error())
	}
	if len(m.checkers) == 0 {
		return mutedStyle.Render("No checks configured yet.")
	}
	if len(m.results) == 0 {
		return fmt.Sprintf("%s checking configured targets...", m.spinner.View())
	}

	services := filterResults(m.results, "service")
	apis := filterResults(m.results, "api")
	summary := m.renderSummaryBar()
	panels := m.renderDashboardColumns([][]dashboardCard{
		{
			{Sections: []dashboardSection{
				{Title: "Services", Results: services, AlwaysShow: true},
			}},
			{Sections: []dashboardSection{
				{Title: "Proxmox Health", Results: filterResults(m.results, "proxmox-health")},
				{Title: "Proxmox VMs", Results: filterResults(m.results, "proxmox-vms")},
			}},
		},
		{
			{Sections: []dashboardSection{
				{Title: "APIs", Results: apis, AlwaysShow: true},
			}},
			{Sections: []dashboardSection{
				{Title: "PBS Health", Results: filterResults(m.results, "pbs-health")},
				{Title: "PBS Datastore Details", Results: filterResults(m.results, "pbs-details")},
			}},
		},
		{
			{Sections: []dashboardSection{
				{Title: "Docker Containers", Results: filterResults(m.results, "docker")},
			}},
		},
	})

	return lipgloss.JoinVertical(lipgloss.Left, summary, panels)
}

type dashboardCard struct {
	Sections []dashboardSection
}

type dashboardSection struct {
	Title      string
	Results    []checks.Result
	AlwaysShow bool
}

func (m model) renderSummaryBar() string {
	okCount, warnCount, errorCount := statusCounts(m.results)
	total := len(m.results)
	width := max(42, min(m.width, 110))

	parts := []string{
		summaryBadgeStyle(checks.StatusOK).Render(fmt.Sprintf(" up %d ", okCount)),
		summaryBadgeStyle(checks.StatusWarning).Render(fmt.Sprintf(" warn %d ", warnCount)),
		summaryBadgeStyle(checks.StatusError).Render(fmt.Sprintf(" down %d ", errorCount)),
	}

	text := lipgloss.JoinHorizontal(lipgloss.Center, parts[0], " ", parts[1], " ", parts[2])
	if weather := weatherSummary(m.results); weather != "" {
		text = lipgloss.JoinHorizontal(lipgloss.Center, text, "  ", weatherStyle.Render(weather))
	}
	right := mutedStyle.Render(fmt.Sprintf("%d checks  refresh %s", total, m.refresh))
	gap := strings.Repeat(" ", max(1, width-lipgloss.Width(text)-lipgloss.Width(right)-2))

	return summaryBarStyle.Width(width).Render(text + gap + right)
}

func (m model) renderDashboardColumns(groups [][]dashboardCard) string {
	visibleGroups := make([][]dashboardCard, 0, len(groups))
	for _, group := range groups {
		visible := make([]dashboardCard, 0, len(group))
		for _, card := range group {
			if len(card.visibleSections()) > 0 {
				visible = append(visible, card)
			}
		}
		if len(visible) > 0 {
			visibleGroups = append(visibleGroups, visible)
		}
	}
	if len(visibleGroups) == 0 {
		return ""
	}

	columns := min(len(visibleGroups), 1)
	if m.width >= 132 {
		columns = min(len(visibleGroups), 3)
	} else if m.width >= 88 {
		columns = min(len(visibleGroups), 2)
	}

	gap := 2
	panelOuterOverhead := 6
	available := max(1, m.width-(columns-1)*gap)
	width := clamp((available/columns)-panelOuterOverhead, 30, 68)

	if columns == 1 {
		all := []string{}
		for _, group := range visibleGroups {
			for _, card := range group {
				all = append(all, m.renderPanel(card, width))
			}
		}
		return strings.Join(all, "\n")
	}

	renderedColumns := make([]string, 0, columns*2-1)
	for i := 0; i < columns; i++ {
		renderedColumns = append(renderedColumns, m.renderCardColumn(visibleGroups[i], width))
		if i < columns-1 {
			renderedColumns = append(renderedColumns, strings.Repeat(" ", gap))
		}
	}
	if len(visibleGroups) > columns {
		overflow := []string{}
		for _, group := range visibleGroups[columns:] {
			overflow = append(overflow, m.renderCardColumn(group, width))
		}
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...),
			strings.Join(overflow, "\n"),
		)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...)
}

func (m model) renderCardColumn(cards []dashboardCard, width int) string {
	rendered := make([]string, 0, len(cards))
	for _, card := range cards {
		rendered = append(rendered, m.renderPanel(card, width))
	}
	return strings.Join(rendered, "\n")
}

func (m model) renderPanel(card dashboardCard, width int) string {
	sections := card.visibleSections()
	lines := []string{}
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			lines = append(lines, "")
		}

		titleText := fmt.Sprintf("%s %s", section.Title, countStyle.Render(fmt.Sprintf("%d", len(section.Results))))
		lines = append(lines, panelTitleStyle(section.Title).Render(titleText))
		if len(section.Results) == 0 {
			lines = append(lines, mutedStyle.Render("No targets configured."))
			continue
		}
		for _, result := range section.Results {
			lines = append(lines, renderResult(result, width-4))
			lines = append(lines, "")
		}
		lines = lines[:len(lines)-1]
	}

	return panelStyle(card.accentTitle()).Width(width).Render(strings.Join(lines, "\n"))
}

func (c dashboardCard) visibleSections() []dashboardSection {
	visible := make([]dashboardSection, 0, len(c.Sections))
	for _, section := range c.Sections {
		if section.AlwaysShow || len(section.Results) > 0 {
			visible = append(visible, section)
		}
	}
	return visible
}

func (c dashboardCard) accentTitle() string {
	for _, section := range c.Sections {
		if section.AlwaysShow || len(section.Results) > 0 {
			return section.Title
		}
	}
	if len(c.Sections) > 0 {
		return c.Sections[0].Title
	}
	return ""
}

func renderResult(result checks.Result, width int) string {
	dot := statusDotStyle(result.Status).Render("●")
	status := statusTextStyle(result.Status).Render(statusLabel(result.Status))
	name := nameStyle.Render(truncate(result.Name, 18))
	latency := latencyStyle.Render(result.Latency.Truncate(time.Millisecond).String())
	checkedAt := timeStyle.Render(result.CheckedAt.Format("15:04:05"))

	metaWidth := lipgloss.Width(latency) + lipgloss.Width(checkedAt) + 3
	if result.Status == checks.StatusError {
		metaWidth = lipgloss.Width(latency) + 2
		checkedAt = ""
	}
	summaryWidth := max(12, width-lipgloss.Width(dot)-lipgloss.Width(status)-lipgloss.Width(name)-metaWidth-8)
	summary := summaryStyle.Width(summaryWidth).Render(truncate(result.Summary, summaryWidth))

	firstLineParts := []string{
		dot,
		" ",
		status,
		" ",
		name,
		"  ",
		summary,
		"  ",
		latency,
	}
	if checkedAt != "" {
		firstLineParts = append(firstLineParts, " ", checkedAt)
	}
	firstLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		firstLineParts...,
	)

	detailParts := []string{}
	if result.Kind == "service" {
		detailParts = append(detailParts, result.Target)
	}
	if len(result.Details) > 0 {
		detailParts = append(detailParts, result.Details...)
	}
	if len(detailParts) == 0 {
		return firstLine
	}

	detailWidth := max(8, width-4)
	detailLines := make([]string, 0, len(detailParts))
	for _, detail := range detailParts {
		for lineIndex, line := range strings.Split(detail, "\n") {
			margin := 3
			if lineIndex > 0 {
				margin = 7
			}
			if result.Kind == "docker" && lineIndex > 0 {
				margin = 6
			}
			detailLines = append(detailLines, detailStyle.
				MarginLeft(margin).
				Width(detailWidth).
				Render(truncate(line, max(8, detailWidth-margin+3))))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, append([]string{firstLine}, detailLines...)...)
}

func statusCounts(results []checks.Result) (int, int, int) {
	var okCount, warnCount, errorCount int
	for _, result := range results {
		switch result.Status {
		case checks.StatusOK:
			okCount++
		case checks.StatusWarning:
			warnCount++
		default:
			errorCount++
		}
	}
	return okCount, warnCount, errorCount
}

func weatherSummary(results []checks.Result) string {
	for _, result := range results {
		if result.Kind == "weather" && result.Summary != "" {
			return result.Summary
		}
	}
	return ""
}

func statusLabel(status checks.Status) string {
	switch status {
	case checks.StatusOK:
		return "UP"
	case checks.StatusWarning:
		return "WARN"
	default:
		return "DOWN"
	}
}

func filterResults(results []checks.Result, kind string) []checks.Result {
	var filtered []checks.Result
	for _, result := range results {
		if result.Kind == kind {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func truncate(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	return value[:width-3] + "..."
}

func clamp(value, minValue, maxValue int) int {
	return min(max(value, minValue), maxValue)
}
