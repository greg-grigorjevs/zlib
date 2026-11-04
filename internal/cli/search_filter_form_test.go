package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	zlib "github.com/heartleo/zlib/internal/zlib"
)

func searchFormStep(m searchFilterForm, msg tea.Msg) searchFilterForm {
	next, _ := m.Update(msg)
	return next.(searchFilterForm)
}

func TestSearchFilterFormEnterSubmitsFocusedQuery(t *testing.T) {
	m := newSearchFilterForm("old query", nil)
	m.input.SetValue("new query")
	m = searchFormStep(m, key("enter"))
	if !m.accepted || m.query != "new query" {
		t.Fatalf("Enter did not submit the edited query: accepted=%v query=%q", m.accepted, m.query)
	}
}

func TestSearchFilterFormEnterValidatesFocusedQuery(t *testing.T) {
	m := newSearchFilterForm("", nil)
	m = searchFormStep(m, key("enter"))
	if m.accepted || m.message == "" || m.cursor != 0 || !m.editing {
		t.Fatalf("empty query should stay focused with a validation error: %+v", m)
	}
}

func TestSearchFilterFormEnterAdvancesYearInput(t *testing.T) {
	m := newSearchFilterForm("books", nil)
	m = searchFormStep(m, key("tab"))
	m.input.SetValue("2001")
	m = searchFormStep(m, key("enter"))
	if m.accepted || m.yearFrom != "2001" || m.cursor != 2 || !m.editing {
		t.Fatalf("Enter should save the year and focus the next field: %+v", m)
	}
}

func TestSearchFilterFormSelectsEveryEAPIConstraint(t *testing.T) {
	m := newSearchFilterForm("books", nil)
	m.yearFrom, m.yearTo = "2001", "2020"
	for range 3 {
		m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	if m.cursor != 3 {
		t.Fatalf("focus = %d, want languages", m.cursor)
	}
	m = searchFormStep(m, key("enter"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeySpace})
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeySpace})
	m = searchFormStep(m, key("esc"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = searchFormStep(m, key("enter"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeySpace})
	m = searchFormStep(m, key("esc"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeySpace})
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m = searchFormStep(m, key("enter"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = searchFormStep(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = searchFormStep(m, key("enter"))
	m = searchFormStep(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !m.accepted {
		t.Fatalf("form rejected filters: %s", m.message)
	}
	opts := m.searchOptions()
	if opts.FromYear != 2001 || opts.ToYear != 2020 || !opts.Exact || opts.Order != zlib.OrderBestMatch ||
		!slices.Equal(opts.Languages, []zlib.Language{zlib.LangEnglish, zlib.LangChinese}) ||
		!slices.Equal(opts.Extensions, []zlib.Extension{zlib.ExtEPUB}) {
		t.Fatalf("search options = %+v", opts)
	}
}

func TestSearchFilterFormValidatesYearsAndFits(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 16}, {30, 12}} {
		m := searchFormStep(newSearchFilterForm("books", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if size[1] >= 16 && !strings.Contains(ansi.Strip(view), "Search") {
			t.Fatalf("search action hidden at %v", size)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("line exceeds %v", size)
			}
		}
		m.cursor, m.editing, m.picker = 3, false, 3
		view = m.View().Content
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("picker exceeds %v", size)
			}
		}
		m.picker, m.cursor = -1, 7
		if !strings.Contains(ansi.Strip(m.View().Content), "Search") {
			t.Fatalf("focused Search action is hidden at %v", size)
		}
	}
	m := newSearchFilterForm("books", nil)
	m.yearFrom, m.yearTo = "2021", "2020"
	m = searchFormStep(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.accepted || !strings.Contains(m.message, "before") {
		t.Fatalf("invalid range accepted: %q", m.message)
	}
}

func TestSearchFilterFormFeedsPagedEAPIRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		for key, want := range map[string]string{"message": "books", "page": "2", "limit": "50", "yearFrom": "2001", "yearTo": "2020", "languages[]": "chinese", "extensions[]": "epub", "e": "1", "order": "bestmatch"} {
			if got := r.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		fmt.Fprint(w, `{"success":1,"books":[],"pagination":{"current":2,"total_pages":3}}`)
	}))
	defer server.Close()
	client := zlib.NewClient(zlib.WithDomain(server.URL))
	client.SetCookies(map[string]string{"remix_userid": "1", "remix_userkey": "k"})
	form := newSearchFilterForm("books", &zlib.SearchOptions{
		FromYear: 2001, ToYear: 2020, Languages: []zlib.Language{zlib.LangChinese},
		Extensions: []zlib.Extension{zlib.ExtEPUB}, Exact: true, Order: zlib.OrderBestMatch,
	})
	form = searchFormStep(form, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !form.accepted {
		t.Fatalf("form rejected: %s", form.message)
	}
	model := newSearchSelectModel(form.query, client, form.searchOptions(), false)
	if _, ok := model.fetchPage(2)().(searchResultMsg); !ok {
		t.Fatal("paged search failed")
	}
}

func TestSearchRefineKeepsAndAppliesFilters(t *testing.T) {
	opts := &zlib.SearchOptions{FromYear: 2010, ToYear: 2020, Languages: []zlib.Language{zlib.LangChinese}, Extensions: []zlib.Extension{zlib.ExtPDF}, Exact: true, Order: zlib.OrderDate}
	m := newSearchSelectModel("books", nil, opts, false)
	m.state = searchStateResults
	m.page, m.totalPages = 3, 5
	m = step(m, key("/"))
	if m.state != searchStateFilter || m.filter.query != "books" || m.filter.order != zlib.OrderDate ||
		m.filter.yearFrom != "2010" || m.filter.yearTo != "2020" || !m.filter.exact ||
		!slices.Equal(m.filter.languages, opts.Languages) || !slices.Equal(m.filter.extensions, opts.Extensions) {
		t.Fatalf("refine form lost filters: %+v", m.filter)
	}
	m.filter.order = zlib.OrderBestMatch
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.state != searchStateLoading || m.page != 1 || m.opts.Order != zlib.OrderBestMatch || !m.opts.Exact {
		t.Fatalf("refine did not apply filters: state=%v page=%d opts=%+v", m.state, m.page, m.opts)
	}
}

func TestSearchFilterFormAlignsLabelsAndShowsErrorsAtField(t *testing.T) {
	opts := &zlib.SearchOptions{Languages: []zlib.Language{zlib.LangEnglish}, Extensions: []zlib.Extension{zlib.ExtEPUB}}
	for _, size := range [][2]int{{120, 30}, {80, 24}, {50, 18}, {30, 14}} {
		m := searchFormStep(newSearchFilterForm("golang", opts), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View().Content)
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("form line exceeds %v: %q", size, ansi.Strip(line))
			}
		}
		if !strings.Contains(view, "Year from   Any") || !strings.Contains(view, "Languages   English") {
			t.Fatalf("filter labels and values are not in columns at %v:\n%s", size, view)
		}
		if !strings.Contains(view, "Filters ─") || !strings.Contains(view, "2 active") {
			t.Fatalf("filter section header missing at %v", size)
		}
		m.yearFrom = "20x"
		m = searchFormStep(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		field := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, "Year from") })
		if m.accepted || field < 0 || field+1 >= len(lines) || !strings.Contains(lines[field+1], symbolError) {
			t.Fatalf("year error is not below its field at %v:\n%s", size, strings.Join(lines, "\n"))
		}
		if errorText := strings.Join(strings.Fields(strings.Join(lines[field+1:], "")), ""); !strings.Contains(errorText, "four-digityear") {
			t.Fatalf("year error was truncated at %v:\n%s", size, strings.Join(lines, "\n"))
		}
	}
}

func TestSearchFilterFormQueryBoxFollowsTerminalSize(t *testing.T) {
	tall := searchFormStep(newSearchFilterForm("golang", nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(ansi.Strip(tall.View().Content), "╭") {
		t.Fatal("tall terminal should frame the query")
	}
	short := searchFormStep(newSearchFilterForm("golang", nil), tea.WindowSizeMsg{Width: 80, Height: 16})
	if strings.Contains(ansi.Strip(short.View().Content), "╭") {
		t.Fatal("short terminal should spend the query frame rows on filters")
	}
	wide := searchFormStep(newSearchFilterForm("golang", nil), tea.WindowSizeMsg{Width: 160, Height: 30})
	for _, line := range strings.Split(wide.View().Content, "\n") {
		line = strings.TrimRight(ansi.Strip(line), " ")
		if strings.Contains(line, "╭") && ansi.StringWidth(line) > 100 {
			t.Fatalf("query frame should stop at a readable width: %d", ansi.StringWidth(line))
		}
	}
}

func TestSearchStatesShowActiveFilters(t *testing.T) {
	opts := &zlib.SearchOptions{FromYear: 2010, ToYear: 2020, Languages: []zlib.Language{zlib.LangEnglish}, Extensions: []zlib.Extension{zlib.ExtEPUB}, Exact: true, Order: zlib.OrderPopular}
	if got := strings.Join(searchFilterChips(opts), " · "); got != "2010–2020 · English · EPUB · Exact · Popular" {
		t.Fatalf("filter summary = %q", got)
	}
	if searchFilterChips(&zlib.SearchOptions{}) != nil {
		t.Fatal("unset filters should not produce a summary")
	}
	m := newSearchSelectModel("golang", nil, opts, false)
	m.width, m.height = 80, 24
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Searching…") || !strings.Contains(view, "English · EPUB") {
		t.Fatalf("loading lacks context:\n%s", view)
	}
	m.state, m.totalPages = searchStateResults, 1
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Filters: 2010–2020") || !strings.Contains(view, "loosen the filters") {
		t.Fatalf("empty result does not explain active filters:\n%s", view)
	}
	m.books = []zlib.Book{{ID: "1", Hash: "h", Name: "Book"}}
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 16}, {30, 14}} {
		m.width, m.height = size[0], size[1]
		lines := strings.Split(m.View().Content, "\n")
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("result line exceeds %v: %q", size, ansi.Strip(line))
			}
		}
		if size[0] >= 80 && !strings.Contains(ansi.Strip(lines[1]), "2010–2020 · English") {
			t.Fatalf("result status lacks filters at %v: %q", size, ansi.Strip(lines[1]))
		}
	}
	m.err, m.retryPage = errors.New("Get https://example.test/eapi/book/search: dial tcp: i/o timeout"), 1
	m.width, m.height = 40, 16
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "i/o timeout") {
		t.Fatalf("narrow error lost its reason:\n%s", view)
	}
}

func TestSearchRefineSizesQueryToTerminal(t *testing.T) {
	m := newSearchSelectModel("golang", nil, nil, false)
	m.state, m.width, m.height = searchStateResults, 40, 16
	m = step(m, key("/"))
	for _, line := range strings.Split(m.View().Content, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("refine form exceeds terminal: %q", ansi.Strip(line))
		}
		if strings.Contains(ansi.Strip(line), "golang") && strings.HasSuffix(strings.TrimRight(ansi.Strip(line), " "), "…") {
			t.Fatalf("query input was sized for another width: %q", ansi.Strip(line))
		}
	}
}
