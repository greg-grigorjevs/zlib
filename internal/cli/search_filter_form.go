package cli

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	keys "charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zlib "github.com/heartleo/zlib/internal/zlib"
)

type searchSort struct {
	label string
	value zlib.OrderOption
}

var searchSorts = []searchSort{
	{"Default", ""}, {"Popular", zlib.OrderPopular}, {"Best match", zlib.OrderBestMatch},
	{"Recently added", zlib.OrderDate}, {"Title A–Z", zlib.OrderTitleAZ},
	{"Title Z–A", zlib.OrderTitleZA}, {"Year", zlib.OrderYear},
	{"Largest file", zlib.OrderFileSizeDesc}, {"Smallest file", zlib.OrderFileSizeAsc},
}

var searchLanguages = []zlib.Language{
	zlib.LangEnglish, zlib.LangChinese, zlib.LangRussian, zlib.LangFrench,
	zlib.LangGerman, zlib.LangSpanish, zlib.LangJapanese, zlib.LangKorean,
	zlib.LangItalian, zlib.LangPortuguese, zlib.LangArabic, zlib.LangDutch,
	zlib.LangPolish, zlib.LangTurkish, zlib.LangHindi,
}

var searchFormats = []zlib.Extension{
	zlib.ExtEPUB, zlib.ExtPDF, zlib.ExtAZW, zlib.ExtAZW3, zlib.ExtMOBI,
	zlib.ExtFB2, zlib.ExtDJVU, zlib.ExtDJV, zlib.ExtRTF, zlib.ExtTXT, zlib.ExtLIT,
}

const searchFilterFields = 8

// searchFilterForm shows every filter in a compact form. Long choice lists open
// a picker while keeping the other field values in the model.
type searchFilterForm struct {
	width, height                        int
	cursor, picker, choice, choiceOffset int
	input                                textinput.Model
	query, yearFrom, yearTo              string
	languages                            []zlib.Language
	extensions                           []zlib.Extension
	exact                                bool
	order                                zlib.OrderOption
	message                              string
	messageField                         int // field the validation message belongs to
	accepted                             bool
	editing                              bool
}

func newSearchFilterForm(query string, opts *zlib.SearchOptions) searchFilterForm {
	m := searchFilterForm{width: 80, height: 24, picker: -1, query: query}
	m.input = textinput.New()
	m.input.Prompt = "> "
	m.input.Placeholder = "Title, author, or ISBN"
	m.input.KeyMap.Paste.SetKeys("ctrl+v", "ctrl+shift+v", "shift+insert")
	m.input.SetWidth(70)
	if opts != nil {
		m.exact, m.order = opts.Exact, opts.Order
		m.languages = slices.Clone(opts.Languages)
		m.extensions = slices.Clone(opts.Extensions)
		if opts.FromYear > 0 {
			m.yearFrom = strconv.Itoa(opts.FromYear)
		}
		if opts.ToYear > 0 {
			m.yearTo = strconv.Itoa(opts.ToYear)
		}
	}
	m.startEdit()
	return m
}

func (m searchFilterForm) Init() tea.Cmd { return m.input.Focus() }

func (m *searchFilterForm) startEdit() tea.Cmd {
	m.editing = true
	// Years are edited in place, inside the value column of their row.
	m.input.Prompt = ""
	m.input.Placeholder = "Any year"
	if m.cursor == 0 {
		m.input.Prompt = "> "
		m.input.Placeholder = "Title, author, or ISBN"
	}
	m.input.SetWidth(m.inputWidth())
	m.input.SetValue([]string{m.query, m.yearFrom, m.yearTo}[m.cursor])
	m.input.CursorEnd()
	return m.input.Focus()
}

func (m *searchFilterForm) finishEdit() {
	if !m.editing {
		return
	}
	switch m.cursor {
	case 0:
		m.query = m.input.Value()
	case 1:
		m.yearFrom = m.input.Value()
	case 2:
		m.yearTo = m.input.Value()
	}
	if m.cursor == m.messageField {
		m.message = ""
	}
	m.editing = false
	m.input.Blur()
}

func (m searchFilterForm) options() (string, []string) {
	switch m.picker {
	case 3:
		out := make([]string, len(searchLanguages))
		for i, v := range searchLanguages {
			out[i] = languageLabel(v)
		}
		return "Languages", out
	case 4:
		out := make([]string, len(searchFormats))
		for i, v := range searchFormats {
			out[i] = v.String()
		}
		return "Formats", out
	case 6:
		out := make([]string, len(searchSorts))
		for i, v := range searchSorts {
			out[i] = v.label
		}
		return "Sort by", out
	}
	return "", nil
}

func (m searchFilterForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		m.input.SetWidth(m.inputWidth())
		return m, nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if m.picker >= 0 {
			_, options := m.options()
			switch k.String() {
			case "esc", "ctrl+c", "tab":
				m.picker = -1
			case "up", "k":
				m.choice = (m.choice - 1 + len(options)) % len(options)
			case "down", "j":
				m.choice = (m.choice + 1) % len(options)
			case "space", "enter":
				switch m.picker {
				case 3:
					value := searchLanguages[m.choice]
					if i := slices.Index(m.languages, value); i >= 0 {
						m.languages = slices.Delete(m.languages, i, i+1)
					} else {
						m.languages = append(m.languages, value)
					}
				case 4:
					value := searchFormats[m.choice]
					if i := slices.Index(m.extensions, value); i >= 0 {
						m.extensions = slices.Delete(m.extensions, i, i+1)
					} else {
						m.extensions = append(m.extensions, value)
					}
				case 6:
					m.order = searchSorts[m.choice].value
					m.picker = -1
				}
			}
			return m, nil
		}
		switch k.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "ctrl+s":
			m.finishEdit()
			return m, m.submit()
		case "tab", "shift+tab":
			m.finishEdit()
			if k.String() == "tab" {
				m.cursor = (m.cursor + 1) % searchFilterFields
			} else {
				m.cursor = (m.cursor - 1 + searchFilterFields) % searchFilterFields
			}
			if m.cursor <= 2 {
				return m, m.startEdit()
			}
			return m, nil
		case "enter":
			if m.editing {
				m.finishEdit()
				if m.cursor == 0 {
					return m, m.submit()
				}
				m.cursor = (m.cursor + 1) % searchFilterFields
				if m.cursor <= 2 {
					return m, m.startEdit()
				}
				return m, nil
			}
			switch m.cursor {
			case 0, 1, 2:
				return m, m.startEdit()
			case 3, 4, 6:
				m.picker, m.choice, m.choiceOffset = m.cursor, 0, 0
			case 5:
				m.exact = !m.exact
			case 7:
				return m, m.submit()
			}
			return m, nil
		case "space":
			if !m.editing && m.cursor == 5 {
				m.exact = !m.exact
				return m, nil
			}
		case "up", "down":
			if !m.editing {
				if k.String() == "up" {
					m.cursor = (m.cursor - 1 + searchFilterFields) % searchFilterFields
				} else {
					m.cursor = (m.cursor + 1) % searchFilterFields
				}
				return m, nil
			}
		}
	}
	if m.editing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *searchFilterForm) submit() tea.Cmd {
	if strings.TrimSpace(m.query) == "" {
		return m.reject(0, "Enter a title, author, or ISBN")
	}
	from, err := parseSearchYear(m.yearFrom)
	if err != nil {
		return m.reject(1, "Year from must be a four-digit year")
	}
	to, err := parseSearchYear(m.yearTo)
	if err != nil {
		return m.reject(2, "Year to must be a four-digit year")
	}
	if from > 0 && to > 0 && from > to {
		return m.reject(1, "Year from must be before year to")
	}
	m.message = ""
	m.accepted = true
	return tea.Quit
}

// reject keeps the form open and returns focus to the field that needs a fix.
func (m *searchFilterForm) reject(field int, message string) tea.Cmd {
	m.message, m.messageField, m.cursor = message, field, field
	return m.startEdit()
}

func parseSearchYear(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	year, err := strconv.Atoi(value)
	if err != nil || len(value) != 4 || year < 1000 || year > 9999 {
		return 0, errors.New("invalid year")
	}
	return year, nil
}

func (m searchFilterForm) searchOptions() *zlib.SearchOptions {
	from, _ := parseSearchYear(m.yearFrom)
	to, _ := parseSearchYear(m.yearTo)
	return &zlib.SearchOptions{FromYear: from, ToYear: to, Languages: slices.Clone(m.languages), Extensions: slices.Clone(m.extensions), Exact: m.exact, Order: m.order}
}

func (m searchFilterForm) View() tea.View {
	if m.picker >= 0 {
		return m.pickerView()
	}
	width := m.formWidth()
	muted := lipgloss.NewStyle().Foreground(currentTheme.Muted)
	label := lipgloss.NewStyle().Foreground(currentTheme.Muted)
	focusLabel := lipgloss.NewStyle().Foreground(currentTheme.Accent).Bold(true)
	value := lipgloss.NewStyle().Foreground(currentTheme.Title)

	var body []string
	focusStart, focusEnd := 0, 0
	add := func(field int, lines ...string) {
		if field == m.cursor {
			focusStart = len(body)
		}
		body = append(body, lines...)
		if field == m.messageField && m.message != "" {
			body = append(body, m.fieldError(width)...)
		}
		if field == m.cursor {
			focusEnd = len(body) - 1
		}
	}

	// Query: a labelled prompt box, like a chat composer, when rows allow it.
	queryLabel := label.Render("Query")
	if m.cursor == 0 {
		queryLabel = focusLabel.Render("Query")
	}
	prompt := value.Render("> " + m.query)
	if m.query == "" {
		prompt = muted.Render("> Title, author, or ISBN")
	}
	if m.cursor == 0 && m.editing {
		prompt = m.input.View()
	}
	if m.queryBoxed() {
		border := currentTheme.Muted
		if m.cursor == 0 {
			border = currentTheme.Accent
		}
		box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
			Padding(0, 1).Width(width).Render(ansi.Truncate(prompt, max(1, width-4), "…"))
		add(0, append([]string{queryLabel}, strings.Split(box, "\n")...)...)
	} else {
		add(0, queryLabel, ansi.Truncate(prompt, width, "…"))
	}

	opts := m.searchOptions()
	active := len(searchFilterChips(opts))
	info := "none set"
	if active > 0 {
		info = fmt.Sprintf("%d active", active)
	}
	body = append(body, "", sectionHeader("Filters", info, width))

	row := func(field int, name, text string, unset bool) {
		focused := field == m.cursor
		name = ansi.Truncate(name, searchLabelWidth-1, "…")
		pad := strings.Repeat(" ", searchLabelWidth-ansi.StringWidth(name))
		switch {
		case focused && m.editing:
			add(field, "  "+focusLabel.Render(name)+pad+m.input.View())
		case focused:
			add(field, styledListRow(name+pad+text, true, width, currentTheme))
		default:
			styled := value.Render(text)
			if unset {
				styled = muted.Render(text)
			}
			add(field, ansi.Truncate("  "+label.Render(name)+pad+styled, width, "…"))
		}
	}
	row(1, "Year from", cmp.Or(m.yearFrom, "Any"), m.yearFrom == "")
	row(2, "Year to", cmp.Or(m.yearTo, "Any"), m.yearTo == "")
	langs := make([]string, len(m.languages))
	for i, v := range m.languages {
		langs[i] = languageLabel(v)
	}
	row(3, "Languages", cmp.Or(strings.Join(langs, ", "), "Any"), len(langs) == 0)
	fmts := make([]string, len(m.extensions))
	for i, v := range m.extensions {
		fmts[i] = v.String()
	}
	row(4, "Formats", cmp.Or(strings.Join(fmts, ", "), "Any"), len(fmts) == 0)
	exact := "[ ]"
	if m.exact {
		exact = "[x]"
	}
	row(5, "Exact match", exact, !m.exact)
	row(6, "Sort by", sortLabel(m.order), m.order == "")
	body = append(body, "")
	add(7, renderActionButton("Search", m.cursor == 7, false))

	visible := max(1, m.height-6)
	if len(body) > visible {
		// Keep the whole focused field, including an inline error, in view.
		start := min(max(0, focusEnd-visible+1), focusStart, len(body)-visible)
		body = body[start : start+visible]
	}
	enterHelp := "edit / select"
	if m.cursor == 0 && m.editing {
		enterHelp = "search"
	}
	return screenView(m.width, m.height, "Search books", "Set filters, then search", strings.Join(body, "\n"),
		ansi.Truncate(searchFieldHelp[m.cursor], max(1, m.width), "…"),
		uiKey("ctrl+s", "search"), uiKey("tab", "next"), uiKey("enter", enterHelp), uiKey("esc", "back"))
}

// searchLabelWidth is the label column width for filter rows, gap included.
const searchLabelWidth = 12

var searchFieldHelp = [searchFilterFields]string{
	"Required · title, author, or ISBN",
	"Four-digit year · leave blank for any",
	"Four-digit year · leave blank for any",
	"Enter to choose · pick several or none",
	"Enter to choose · pick several or none",
	"Space toggles · match the query as a phrase",
	"Enter to choose the result order",
	"Enter or Ctrl+S runs the search",
}

// formWidth caps the form so long rows stay readable on wide terminals.
func (m searchFilterForm) formWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return max(1, min(width, 100))
}

// queryBoxed reports whether the query fits in a bordered box; short or
// narrow terminals spend those two rows on filters instead.
func (m searchFilterForm) queryBoxed() bool {
	return m.height >= 20 && m.formWidth() >= 24
}

func (m searchFilterForm) inputWidth() int {
	width := m.formWidth()
	if m.cursor != 0 {
		return max(1, width-2-searchLabelWidth-1)
	}
	if m.queryBoxed() {
		width -= 4
	}
	// One cell for the cursor and one spare so the input never touches the edge.
	return max(1, width-ansi.StringWidth(m.input.Prompt)-2)
}

// fieldError wraps instead of truncating: an error must stay readable at
// any width. Narrow terminals drop the label-column indent to make room.
func (m searchFilterForm) fieldError(width int) []string {
	indent := "  "
	if m.messageField > 0 && width-searchLabelWidth-2 >= 24 {
		indent += strings.Repeat(" ", searchLabelWidth)
	}
	style := lipgloss.NewStyle().Foreground(currentTheme.Error)
	return wrapIndented(symbolError+" "+m.message, width, indent, style)
}

// sectionHeader follows Crush's section rule: a title, a line filling the
// remaining width, then optional muted information.
func sectionHeader(title, info string, width int) string {
	accent := lipgloss.NewStyle().Foreground(currentTheme.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(currentTheme.Muted)
	line := width - ansi.StringWidth(title) - 1
	if info != "" {
		line -= ansi.StringWidth(info) + 1
	}
	if line < 2 {
		return ansi.Truncate(accent.Render(title), width, "…")
	}
	out := accent.Render(title) + " " + muted.Render(strings.Repeat("─", line))
	if info != "" {
		out += " " + muted.Render(info)
	}
	return out
}

func languageLabel(language zlib.Language) string {
	if language == "" {
		return ""
	}
	return strings.ToUpper(string(language[:1])) + string(language[1:])
}

func sortLabel(order zlib.OrderOption) string {
	for _, item := range searchSorts {
		if item.value == order {
			return item.label
		}
	}
	return string(order)
}

// searchFilterChips lists the active filters in form order, for the result,
// loading, and empty states. It is empty when no filter narrows the search.
func searchFilterChips(opts *zlib.SearchOptions) []string {
	if opts == nil {
		return nil
	}
	var chips []string
	switch {
	case opts.FromYear > 0 && opts.ToYear > 0:
		chips = append(chips, fmt.Sprintf("%d–%d", opts.FromYear, opts.ToYear))
	case opts.FromYear > 0:
		chips = append(chips, fmt.Sprintf("From %d", opts.FromYear))
	case opts.ToYear > 0:
		chips = append(chips, fmt.Sprintf("Until %d", opts.ToYear))
	}
	for _, language := range opts.Languages {
		chips = append(chips, languageLabel(language))
	}
	for _, extension := range opts.Extensions {
		chips = append(chips, extension.String())
	}
	if opts.Exact {
		chips = append(chips, "Exact")
	}
	if opts.Order != "" {
		chips = append(chips, sortLabel(opts.Order))
	}
	return chips
}

func (m searchFilterForm) pickerView() tea.View {
	title, options := m.options()
	if m.width >= 34 && m.height >= 13 {
		return m.pickerOverlayView(title, options)
	}
	visible := max(1, min(len(options), m.height-7))
	start := min(max(0, m.choiceOffset), max(0, len(options)-visible))
	if m.choice < start {
		start = m.choice
	} else if m.choice >= start+visible {
		start = m.choice - visible + 1
	}
	rows := make([]string, 0, visible)
	for i := start; i < min(len(options), start+visible); i++ {
		mark := "[ ] "
		if m.picker == 3 && slices.Contains(m.languages, searchLanguages[i]) || m.picker == 4 && slices.Contains(m.extensions, searchFormats[i]) || m.picker == 6 && m.order == searchSorts[i].value {
			mark = "[x] "
		}
		rows = append(rows, selectionRow(mark+options[i], i == m.choice, m.width))
	}
	bindings := []keys.Binding{uiKey("↑/↓", "choose"), uiKey("space", "toggle"), uiKey("enter", "select"), uiKey("esc", "done")}
	if m.picker == 6 {
		bindings = []keys.Binding{uiKey("↑/↓", "choose"), uiKey("enter", "select"), uiKey("esc", "back")}
	}
	return screenView(m.width, m.height, title, "", strings.Join(rows, "\n"), "", bindings...)
}

func (m searchFilterForm) pickerOverlayView(title string, options []string) tea.View {
	inner := min(58, m.width-2) - 4
	visible := max(1, min(len(options), min(17, m.height-2)-8))
	start := min(max(0, m.choice-visible+1), max(0, len(options)-visible))
	// Everything inside the panel paints the Surface background itself: an
	// unstyled run after an inner reset shows the terminal's background as a
	// dark band.
	onSurface := lipgloss.NewStyle().Background(currentTheme.Surface)
	rows := []string{onSurface.Foreground(currentTheme.Accent).Bold(true).Render(title), ""}
	for i := start; i < min(len(options), start+visible); i++ {
		mark := "[ ] "
		if m.picker == 3 && slices.Contains(m.languages, searchLanguages[i]) ||
			m.picker == 4 && slices.Contains(m.extensions, searchFormats[i]) ||
			m.picker == 6 && m.order == searchSorts[i].value {
			mark = "[x] "
		}
		style := lipgloss.NewStyle().Foreground(currentTheme.Title).Background(currentTheme.Surface).Width(inner)
		if i == m.choice {
			style = style.Foreground(currentTheme.Surface).Background(currentTheme.Accent).Bold(true)
		}
		rows = append(rows, style.Render(ansi.Truncate(mark+options[i], inner, "…")))
	}
	for len(rows) < visible+2 {
		rows = append(rows, "")
	}
	// Plain text like the other dialogs' hints: help.ShortHelpView leaves the
	// space after each key unstyled, which showed as a dark gap on the panel.
	hints := "↑/↓ choose · space toggle · esc done"
	if m.picker == 6 {
		hints = "↑/↓ choose · enter select · esc back"
	}
	rows = append(rows, "", onSurface.Foreground(currentTheme.Muted).Render(ansi.Truncate(hints, inner, "…")))
	panel := lipgloss.NewStyle().Background(currentTheme.Surface).Width(inner).Padding(1, 1).
		Border(lipgloss.RoundedBorder()).BorderForeground(currentTheme.Accent).Render(strings.Join(rows, "\n"))
	background := m
	background.picker = -1
	base := lipgloss.NewStyle().Foreground(currentTheme.Muted).Render(ansi.Strip(background.View().Content))
	x := max(0, (m.width-lipgloss.Width(panel))/2)
	y := max(0, (m.height-lipgloss.Height(panel))/2)
	return fullScreenView(m.width, m.height, lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(panel).X(x).Y(y)).Render())
}

func runSearchFilterForm(m searchFilterForm) (searchFilterForm, error) {
	if !isInteractive() {
		return m, fmt.Errorf("search requires a terminal")
	}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return m, err
	}
	return result.(searchFilterForm), nil
}
