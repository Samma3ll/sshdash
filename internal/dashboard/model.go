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
	cfg      config.Config
	cfgPath  string
	checkers []checks.Checker
	results  []checks.Result
	refresh  time.Duration
	spinner  spinner.Model
	width    int
	height   int
	err      error
	tab      int
	mode     appMode
	settings settingsModel
	status   string
	selected int
	detail   detailPage
	hitboxes []cardHitbox
}

type checkResultsMsg []checks.Result

type errMsg struct {
	err error
}

type settingsSavedMsg struct {
	cfg config.Config
	err error
}

type appMode int

const (
	modeDashboard appMode = iota
	modeSettings
)

type detailPage string

const (
	detailNone     detailPage = ""
	detailServices detailPage = "services"
	detailAPIs     detailPage = "apis"
	detailProxmox  detailPage = "proxmox"
	detailPBS      detailPage = "pbs"
	detailDocker   detailPage = "docker"
)

var dashboardTabs = []string{"Overview", "Media"}

func NewProgram(cfg config.Config, cfgPath string) func(ssh.Session) (tea.Model, []tea.ProgramOption) {
	return func(_ ssh.Session) (tea.Model, []tea.ProgramOption) {
		lipgloss.SetColorProfile(termenv.ANSI256)

		return newModel(cfg, cfgPath), []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
	}
}

func newModel(cfg config.Config, cfgPath string) model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = spinnerStyle

	return model{
		cfg:      cfg,
		cfgPath:  cfgPath,
		checkers: checks.FromConfig(cfg),
		refresh:  cfg.Refresh,
		spinner:  spin,
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
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == modeSettings {
			return m.updateSettings(msg)
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "s":
			m.mode = modeSettings
			m.settings = newSettingsModel(m.cfg)
			m.status = ""
			return m, nil
		case "r":
			m.status = ""
			return m, m.runChecks()
		case "esc", "backspace":
			if m.detail != detailNone {
				m.detail = detailNone
				return m, nil
			}
		case "enter":
			if m.detail == detailNone && m.isOverviewTab() {
				m.openSelectedCard()
			}
			return m, nil
		case "tab", "right", "down", "l", "j":
			if m.detail == detailNone && m.isOverviewTab() && m.moveSelectedCard(1) {
				return m, nil
			}
			if msg.String() == "tab" || msg.String() == "right" || msg.String() == "l" {
				m.tab = (m.tab + 1) % len(dashboardTabs)
				m.detail = detailNone
			}
			return m, nil
		case "shift+tab", "left", "up", "h", "k":
			if m.detail == detailNone && m.isOverviewTab() && m.moveSelectedCard(-1) {
				return m, nil
			}
			if msg.String() == "shift+tab" || msg.String() == "left" || msg.String() == "h" {
				m.tab = (m.tab + len(dashboardTabs) - 1) % len(dashboardTabs)
				m.detail = detailNone
			}
			return m, nil
		case "1":
			m.tab = 0
			m.detail = detailNone
			return m, nil
		case "2":
			m.tab = 1
			m.detail = detailNone
			return m, nil
		}
	case tea.MouseMsg:
		if m.mode == modeDashboard && m.detail == detailNone && m.isOverviewTab() {
			event := tea.MouseEvent(msg)
			if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress {
				if page, ok := m.cardAt(event.X, event.Y); ok {
					m.detail = page
					return m, nil
				}
			}
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
	case settingsSavedMsg:
		if msg.err != nil {
			m.settings.notice = "Save failed: " + msg.err.Error()
			return m, nil
		}
		m.cfg = msg.cfg
		m.refresh = msg.cfg.Refresh
		m.checkers = checks.FromConfig(msg.cfg)
		m.results = nil
		m.err = nil
		m.status = "settings saved"
		m.mode = modeDashboard
		m.detail = detailNone
		m.selected = 0
		return m, m.runChecks()
	case tickMsg:
		return m, m.runChecks()
	}

	return m, nil
}

func (m model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	next, action := m.settings.Update(msg)
	m.settings = next

	switch action {
	case settingsActionClose:
		m.mode = modeDashboard
		return m, nil
	case settingsActionSave:
		m.status = ""
		return m, m.saveSettings()
	default:
		return m, nil
	}
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
	if m.mode == modeSettings {
		body := m.settings.View(m.width, m.height)
		help := helpStyle.Render("tab sections  up/down field  enter edit/toggle  a add  d delete  h header  ctrl+s save  esc back")
		return m.renderWithFooter([]string{header, "", body}, help)
	}

	tabs := m.renderTabs()
	helpText := "tab/arrows select  enter open  1 overview  2 media  r refresh  s settings  q quit"
	if m.detail != detailNone {
		helpText = "esc back  r refresh  s settings  q quit"
	} else if !m.isOverviewTab() {
		helpText = "tab switch  1 overview  2 media  r refresh  s settings  q quit"
	}
	help := helpStyle.Render(helpText)
	body := m.renderBody()
	parts := []string{header, tabs}
	if m.status != "" {
		parts = append(parts, successStyle.Render(m.status))
	}
	parts = append(parts, "", body)

	return m.renderWithFooter(parts, help)
}

func (m model) renderWithFooter(parts []string, footer string) string {
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if m.height <= 0 {
		return lipgloss.JoinVertical(lipgloss.Left, content, "", footer)
	}

	footerHeight := lipgloss.Height(footer)
	bodyHeight := max(0, m.height-footerHeight)
	body := fitHeight(content, bodyHeight)
	if body == "" {
		return footer
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func fitHeight(content string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
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

func (m model) saveSettings() tea.Cmd {
	pending := m.settings.pending
	path := m.cfgPath
	return func() tea.Msg {
		if err := config.Save(path, pending); err != nil {
			return settingsSavedMsg{err: err}
		}
		cfg, err := config.Load(path)
		return settingsSavedMsg{cfg: cfg, err: err}
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
	var panels string
	if dashboardTabs[m.tab] == "Media" {
		m.hitboxes = nil
		panels = m.renderMediaPage()
	} else if m.detail != detailNone {
		m.hitboxes = nil
		panels = m.renderDetailPage()
	} else {
		panels = m.renderOverviewPage(services, apis)
	}

	return lipgloss.JoinVertical(lipgloss.Left, summary, panels)
}

func (m model) renderOverviewPage(services []checks.Result, apis []checks.Result) string {
	cards := m.overviewCards(services, apis)
	m.clampSelected(cards)
	return m.renderDashboardColumns([][]dashboardCard{
		{
			cards[0],
			cards[2],
		},
		{
			cards[1],
			cards[3],
		},
		{
			cards[4],
		},
	})
}

func (m model) overviewCards(services []checks.Result, apis []checks.Result) []dashboardCard {
	return []dashboardCard{
		{
			ID:    detailServices,
			Title: "Services",
			Sections: []dashboardSection{
				{Title: "Services", Results: services, AlwaysShow: true},
			},
		},
		{
			ID:    detailAPIs,
			Title: "APIs",
			Sections: []dashboardSection{
				{Title: "APIs", Results: apis, AlwaysShow: true},
			},
		},
		{
			ID:    detailProxmox,
			Title: "Proxmox",
			Sections: []dashboardSection{
				{Title: "Proxmox Health", Results: filterResults(m.results, "proxmox-health")},
				{Title: "Proxmox VMs", Results: filterResults(m.results, "proxmox-vms")},
			},
		},
		{
			ID:    detailPBS,
			Title: "PBS",
			Sections: []dashboardSection{
				{Title: "PBS Health", Results: filterResults(m.results, "pbs-health")},
				{Title: "PBS Datastore Details", Results: filterResults(m.results, "pbs-details")},
			},
		},
		{
			ID:    detailDocker,
			Title: "Docker",
			Sections: []dashboardSection{
				{Title: "Docker Containers", Results: filterResults(m.results, "docker")},
			},
		},
	}
}

func (m model) renderDetailPage() string {
	width := max(42, min(m.width, 110))
	switch m.detail {
	case detailServices:
		return m.renderDetailPanel("Services", []dashboardSection{
			{Title: "Services", Results: filterResults(m.results, "service"), AlwaysShow: true},
		}, width)
	case detailAPIs:
		return m.renderDetailPanel("APIs", []dashboardSection{
			{Title: "APIs", Results: filterResults(m.results, "api"), AlwaysShow: true},
		}, width)
	case detailProxmox:
		return m.renderDetailPanel("Proxmox", []dashboardSection{
			{Title: "Proxmox Health", Results: filterResults(m.results, "proxmox-health")},
			{Title: "Proxmox VMs", Results: filterResults(m.results, "proxmox-vms")},
		}, width)
	case detailPBS:
		return m.renderDetailPanel("PBS", []dashboardSection{
			{Title: "PBS Health", Results: filterResults(m.results, "pbs-health")},
			{Title: "PBS Datastore Details", Results: filterResults(m.results, "pbs-details")},
		}, width)
	case detailDocker:
		return m.renderDetailPanel("Docker", []dashboardSection{
			{Title: "Docker Containers", Results: filterResults(m.results, "docker")},
		}, width)
	default:
		return ""
	}
}

func (m model) renderDetailPanel(title string, sections []dashboardSection, width int) string {
	card := dashboardCard{Title: title, Sections: sections}
	if len(card.visibleSections()) == 0 {
		return mutedStyle.Render("No data for " + title + ".")
	}

	lines := []string{detailTitleStyle.Render(title), mutedStyle.Render("Esc/backspace returns to overview."), ""}
	for sectionIndex, section := range card.visibleSections() {
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
			lines = append(lines, renderResult(result, width-8))
			lines = append(lines, "")
		}
		lines = lines[:len(lines)-1]
	}

	return detailPanelStyle(card.accentTitle()).Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderMediaPage() string {
	mediaCard := dashboardCard{Sections: []dashboardSection{
		{Title: "Jellyfin Library", Results: filterResults(m.results, "media-jellyfin")},
		{Title: "Radarr", Results: filterResults(m.results, "media-radarr")},
		{Title: "Sonarr", Results: filterResults(m.results, "media-sonarr")},
		{Title: "Jellyseerr", Results: filterResults(m.results, "media-jellyseerr")},
	}}
	if len(mediaCard.visibleSections()) == 0 {
		return mutedStyle.Render("No media services configured.")
	}

	return m.renderDashboardColumns([][]dashboardCard{
		{mediaCard},
	})
}

func (m model) renderTabs() string {
	width := max(42, min(m.width, 110))
	rendered := make([]string, 0, len(dashboardTabs))
	for index, title := range dashboardTabs {
		label := fmt.Sprintf(" %d %s ", index+1, title)
		style := inactiveTabStyle
		if index == m.tab {
			style = activeTabStyle
		}
		rendered = append(rendered, style.Render(label))
	}
	return tabsStyle.Width(width).Render(lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
}

type dashboardCard struct {
	ID       detailPage
	Title    string
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
	m.hitboxes = nil
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
		y := 0
		for _, group := range visibleGroups {
			for _, card := range group {
				rendered := m.renderPanel(card, width)
				all = append(all, rendered)
				m.recordHitbox(card, 0, y, lipgloss.Width(rendered), lipgloss.Height(rendered))
				y += lipgloss.Height(rendered) + 1
			}
		}
		return strings.Join(all, "\n")
	}

	renderedColumns := make([]string, 0, columns*2-1)
	x := 0
	for i := 0; i < columns; i++ {
		column := m.renderCardColumn(visibleGroups[i], width, x)
		renderedColumns = append(renderedColumns, column)
		if i < columns-1 {
			renderedColumns = append(renderedColumns, strings.Repeat(" ", gap))
		}
		x += lipgloss.Width(column) + gap
	}
	if len(visibleGroups) > columns {
		overflow := []string{}
		for _, group := range visibleGroups[columns:] {
			overflow = append(overflow, m.renderCardColumn(group, width, 0))
		}
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...),
			strings.Join(overflow, "\n"),
		)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...)
}

func (m model) renderCardColumn(cards []dashboardCard, width int, x int) string {
	rendered := make([]string, 0, len(cards))
	y := 0
	for _, card := range cards {
		panel := m.renderPanel(card, width)
		rendered = append(rendered, panel)
		m.recordHitbox(card, x, y, lipgloss.Width(panel), lipgloss.Height(panel))
		y += lipgloss.Height(panel) + 1
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
			if card.ID != detailNone {
				lines = append(lines, renderCompactResult(result, width-4))
			} else {
				lines = append(lines, renderResult(result, width-4))
			}
			lines = append(lines, "")
		}
		lines = lines[:len(lines)-1]
	}

	if card.ID != detailNone {
		hint := "enter/click open"
		if m.cardIsSelected(card) {
			hint = "> " + hint
		}
		lines = append(lines, "", openHintStyle(m.cardIsSelected(card)).Render(hint))
	}

	return panelStyle(card.accentTitle(), m.cardIsSelected(card)).Width(width).Render(strings.Join(lines, "\n"))
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
	if c.Title != "" {
		return c.Title
	}
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

func (m model) isOverviewTab() bool {
	return dashboardTabs[m.tab] == "Overview"
}

func (m *model) clampSelected(cards []dashboardCard) {
	count := openableCardCount(cards)
	if count == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= count {
		m.selected = count - 1
	}
}

func (m *model) moveSelectedCard(delta int) bool {
	cards := m.overviewCards(filterResults(m.results, "service"), filterResults(m.results, "api"))
	count := openableCardCount(cards)
	if count == 0 {
		m.selected = 0
		return false
	}
	m.selected = (m.selected + delta + count) % count
	return true
}

func (m *model) openSelectedCard() {
	cards := m.overviewCards(filterResults(m.results, "service"), filterResults(m.results, "api"))
	m.clampSelected(cards)
	index := 0
	for _, card := range cards {
		if card.ID == detailNone || len(card.visibleSections()) == 0 {
			continue
		}
		if index == m.selected {
			m.detail = card.ID
			return
		}
		index++
	}
}

func (m model) cardIsSelected(card dashboardCard) bool {
	if card.ID == detailNone || len(card.visibleSections()) == 0 {
		return false
	}
	index := 0
	for _, candidate := range m.overviewCards(filterResults(m.results, "service"), filterResults(m.results, "api")) {
		if candidate.ID == detailNone || len(candidate.visibleSections()) == 0 {
			continue
		}
		if candidate.ID == card.ID {
			return index == m.selected
		}
		index++
	}
	return false
}

func openableCardCount(cards []dashboardCard) int {
	count := 0
	for _, card := range cards {
		if card.ID != detailNone && len(card.visibleSections()) > 0 {
			count++
		}
	}
	return count
}

type cardHitbox struct {
	page   detailPage
	x, y   int
	width  int
	height int
}

func (m *model) recordHitbox(card dashboardCard, x, y, width, height int) {
	if card.ID == detailNone || len(card.visibleSections()) == 0 {
		return
	}
	m.hitboxes = append(m.hitboxes, cardHitbox{
		page:   card.ID,
		x:      x,
		y:      y,
		width:  width,
		height: height,
	})
}

func (m model) cardAt(x, y int) (detailPage, bool) {
	hitboxes := m.hitboxes
	panelY := y - m.overviewPanelOriginY()
	if len(hitboxes) == 0 {
		hitboxes = m.overviewHitboxes()
	}
	for _, hitbox := range hitboxes {
		if x >= hitbox.x && x < hitbox.x+hitbox.width && panelY >= hitbox.y && panelY < hitbox.y+hitbox.height {
			return hitbox.page, true
		}
	}
	return detailNone, false
}

func (m model) overviewPanelOriginY() int {
	y := 3
	if m.status != "" {
		y++
	}
	if len(m.results) > 0 {
		y += lipgloss.Height(m.renderSummaryBar())
	}
	return y
}

func (m model) overviewHitboxes() []cardHitbox {
	cards := m.overviewCards(filterResults(m.results, "service"), filterResults(m.results, "api"))
	groups := [][]dashboardCard{
		{cards[0], cards[2]},
		{cards[1], cards[3]},
		{cards[4]},
	}

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
		return nil
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

	hitboxes := []cardHitbox{}
	if columns == 1 {
		y := 0
		for _, group := range visibleGroups {
			for _, card := range group {
				rendered := m.renderPanel(card, width)
				hitboxes = append(hitboxes, cardHitbox{page: card.ID, x: 0, y: y, width: lipgloss.Width(rendered), height: lipgloss.Height(rendered)})
				y += lipgloss.Height(rendered) + 1
			}
		}
		return hitboxes
	}

	x := 0
	for i := 0; i < columns; i++ {
		y := 0
		columnWidth := 0
		for _, card := range visibleGroups[i] {
			rendered := m.renderPanel(card, width)
			columnWidth = max(columnWidth, lipgloss.Width(rendered))
			hitboxes = append(hitboxes, cardHitbox{page: card.ID, x: x, y: y, width: lipgloss.Width(rendered), height: lipgloss.Height(rendered)})
			y += lipgloss.Height(rendered) + 1
		}
		x += columnWidth + gap
	}
	if len(visibleGroups) > columns {
		yOffset := 0
		for i := 0; i < columns; i++ {
			column := m.renderCardColumn(visibleGroups[i], width, 0)
			yOffset = max(yOffset, lipgloss.Height(column))
		}
		for _, group := range visibleGroups[columns:] {
			y := yOffset
			for _, card := range group {
				rendered := m.renderPanel(card, width)
				hitboxes = append(hitboxes, cardHitbox{page: card.ID, x: 0, y: y, width: lipgloss.Width(rendered), height: lipgloss.Height(rendered)})
				y += lipgloss.Height(rendered) + 1
			}
		}
	}
	return hitboxes
}

func (m model) cardAtPanelPosition(x, y int) (detailPage, bool) {
	for _, hitbox := range m.overviewHitboxes() {
		if x >= hitbox.x && x < hitbox.x+hitbox.width && y >= hitbox.y && y < hitbox.y+hitbox.height {
			return hitbox.page, true
		}
	}
	return detailNone, false
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

func renderCompactResult(result checks.Result, width int) string {
	dot := statusDotStyle(result.Status).Render("●")
	status := statusTextStyle(result.Status).Render(statusLabel(result.Status))
	name := nameStyle.Render(truncate(result.Name, 16))
	latency := latencyStyle.Render(result.Latency.Truncate(time.Millisecond).String())

	firstLineGap := strings.Repeat(" ", max(1, width-lipgloss.Width(dot)-lipgloss.Width(status)-lipgloss.Width(name)-lipgloss.Width(latency)-5))
	firstLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		dot,
		" ",
		status,
		" ",
		name,
		firstLineGap,
		latency,
	)

	summaryWidth := max(10, width-3)
	summary := summaryStyle.Width(summaryWidth).Render(truncate(result.Summary, summaryWidth))
	summaryLine := detailStyle.MarginLeft(3).Width(summaryWidth).Render(summary)

	return lipgloss.JoinVertical(lipgloss.Left, firstLine, summaryLine)
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
