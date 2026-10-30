package cli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zlib "github.com/heartleo/zlib/internal/zlib"
)

func TestSearchViewportMovesOnlyAtItsEdges(t *testing.T) {
	m := feedTestModel()
	m.feed = nil
	m.width, m.height = 80, 24
	for range m.feedRows() {
		m = step(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.listOffset != 1 {
		t.Fatalf("offset after leaving first viewport = %d, want 1", m.listOffset)
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.listOffset != 1 {
		t.Fatal("moving within visible rows shifted the viewport")
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.listOffset != 0 {
		t.Fatal("Home did not bring the first result into view")
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.listOffset != len(m.books)-m.feedRows() {
		t.Fatal("End did not reveal the last result")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	m = next.(searchSelectModel)
	if m.cursor < m.listOffset || m.cursor >= m.listOffset+m.feedRows() {
		t.Fatal("resize hid the selected result")
	}
}

func TestSearchResultPositionAppearsOnce(t *testing.T) {
	m := resultsModel("鬼吹灯")
	m.books = make([]zlib.Book, 50)
	for i := range m.books {
		m.books[i].Name = fmt.Sprintf("Book %d", i+1)
	}
	m.page, m.totalPages, m.cursor = 1, 6, 4
	for _, width := range []int{80, 40, 30} {
		m.width, m.height = width, 24
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if strings.TrimSpace(lines[0]) != "Search: 鬼吹灯" {
			t.Fatalf("width %d: title row carries more than the title: %q", width, lines[0])
		}
		if !strings.Contains(lines[1], "Page 1 of 6 · Book 5 of 50") {
			t.Fatalf("width %d: status row %q lacks the labeled page and position", width, lines[1])
		}
		view := strings.Join(lines, "\n")
		if strings.Count(view, "5 of 50") != 1 || strings.Contains(view, "━") {
			t.Fatalf("width %d repeats the position or draws a progress bar", width)
		}
	}
}

// Every list shows the selected position once, labeled, on the status row.
// Search used to show it three times (highlighted row, an unlabeled "5 / 50"
// beside the title, and a progress bar that read as progress through all
// pages), while feeds and favorites showed it differently.
func TestBookListsShowOneLabeledPosition(t *testing.T) {
	books := []zlib.Book{{ID: "1", Hash: "a", Name: "One"}, {ID: "2", Hash: "b", Name: "Two"}, {ID: "3", Hash: "c", Name: "Three"}}
	feed := newBookFeedSelectModel("Recommended", func() ([]zlib.Book, error) { return books, nil })
	feed.books, feed.state, feed.cursor = books, searchStateResults, 1
	saved := newSavedBooksSelectModel(nil)
	saved.books, saved.state, saved.cursor, saved.page, saved.totalPages = books, searchStateResults, 1, 2, 4
	for _, tc := range []struct {
		name string
		page searchSelectModel
		want string
	}{
		{"recommended", feed, "Book 2 of 3"},
		{"favorites", saved, "Page 2 of 4 · Book 2 of 3"},
	} {
		tc.page.width, tc.page.height = 80, 24
		lines := strings.Split(ansi.Strip(tc.page.View().Content), "\n")
		if !strings.Contains(lines[1], tc.want) || strings.Contains(lines[0], "of 3") {
			t.Errorf("%s header = %q / %q, want %q on the status row only", tc.name, lines[0], lines[1], tc.want)
		}
	}
}

func TestSearchListSpacingFitsTerminal(t *testing.T) {
	m := resultsModel("books")
	m.books = make([]zlib.Book, 50)
	for i := range m.books {
		m.books[i].Name = fmt.Sprintf("Book %d", i+1)
	}
	for _, size := range [][2]int{{80, 24}, {80, 23}, {70, 24}, {40, 24}, {30, 24}, {40, 16}, {40, 15}, {30, 14}} {
		m.width, m.height = size[0], size[1]
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(lines) > m.height {
			t.Fatalf("search list has %d lines in a %d-row terminal", len(lines), m.height)
		}
		header := -1
		for i, line := range lines {
			if strings.Contains(line, "#") && strings.Contains(line, "Title") {
				header = i
				break
			}
		}
		if header < 1 || strings.TrimSpace(lines[header-1]) != "" {
			t.Fatalf("search list lacks space after the selected book at %v", size)
		}
		if m.height >= 15 && (strings.TrimSpace(lines[len(lines)-2]) != "" || strings.TrimSpace(lines[len(lines)-3]) == "") {
			t.Fatalf("search list lacks space before footer at %v", size)
		}
		if m.height == 14 && m.feedRows() != 1 {
			t.Fatal("compact terminal lost its visible result row")
		}
	}
}

func TestBookListSpacingMatchesSearchAcrossPages(t *testing.T) {
	books := make([]zlib.Book, 50)
	history := make([]zlib.DownloadHistoryItem, 50)
	for i := range books {
		books[i] = zlib.Book{ID: fmt.Sprint(i + 1), Name: fmt.Sprintf("Book %d", i+1)}
		history[i] = zlib.DownloadHistoryItem{ID: fmt.Sprint(i + 1), Name: books[i].Name}
	}
	pages := []struct {
		name string
		view func(width, height int) string
	}{
		{"search", func(width, height int) string {
			m := newSearchSelectModel("example", nil, nil, false)
			m.state, m.books, m.width, m.height = searchStateResults, books, width, height
			return ansi.Strip(m.View().Content)
		}},
		{"recommended", func(width, height int) string {
			m := newBookFeedSelectModel("Recommended", nil)
			m.state, m.books, m.width, m.height = searchStateResults, books, width, height
			return ansi.Strip(m.View().Content)
		}},
		{"most-popular", func(width, height int) string {
			m := newBookFeedSelectModel("Most popular", nil)
			m.state, m.books, m.width, m.height = searchStateResults, books, width, height
			return ansi.Strip(m.View().Content)
		}},
		{"favorites", func(width, height int) string {
			m := newSavedBooksSelectModel(nil)
			m.state, m.books, m.width, m.height = searchStateResults, books, width, height
			return ansi.Strip(m.View().Content)
		}},
		{"history", func(width, height int) string {
			m := newHistorySelectModel(nil)
			m.state, m.items, m.width, m.height = historyStateResults, history, width, height
			return ansi.Strip(m.View().Content)
		}},
	}
	for _, size := range [][2]int{{80, 24}, {80, 23}, {70, 24}, {40, 24}, {30, 24}, {40, 16}, {40, 15}, {30, 14}} {
		for _, page := range pages {
			lines := strings.Split(page.view(size[0], size[1]), "\n")
			if len(lines) != size[1] {
				t.Fatalf("%s at %v has %d rows", page.name, size, len(lines))
			}
			header := -1
			for i, line := range lines {
				if strings.Contains(line, "#") && strings.Contains(line, "Title") {
					header = i
					break
				}
			}
			if header != 3+selectedBookCardRows(size[0], size[1]) || strings.TrimSpace(lines[header-1]) != "" {
				t.Fatalf("%s at %v has inconsistent space above the list", page.name, size)
			}
			if size[1] >= 15 && strings.TrimSpace(lines[len(lines)-2]) != "" {
				t.Fatalf("%s at %v has no space below the list", page.name, size)
			}
			if !strings.Contains(lines[len(lines)-1], "q") {
				t.Fatalf("%s at %v lost its footer", page.name, size)
			}
		}
	}
}

func TestSearchCardKeepsMetadataReadable(t *testing.T) {
	book := zlib.Book{Name: "A practical book", Authors: []string{"An Author"}, Extension: "epub", Size: "4 MB", Year: "2024", Language: "English", Publisher: "Example Press", ID: "42", Hash: "abc"}
	card := ansi.Strip(selectedBookCard(book, 40))
	if !strings.Contains(card, "EPUB · 4 MB · 2024") || !strings.Contains(card, "English · Example Press") || !strings.Contains(card, "ID: 42:abc") {
		t.Fatal("selected book card lost metadata")
	}
	if len(strings.Split(card, "\n")) != 8 {
		t.Fatal("selected book card changed height")
	}
}

func TestExpandedBookCardShowsPublicationDetails(t *testing.T) {
	book := zlib.Book{
		ID: "42", Hash: "abc", Name: "亲密关系", Authors: []string{"Author"},
		Categories: "Self-Help, Relationships & Lifestyle - Relationships",
		Year:       "2015", Edition: "6", Publisher: "人民邮电出版社", Language: "Chinese",
		Pages: "540", ISBN10: "7115390576", ISBN13: "9787115390578",
	}
	for _, size := range [][2]int{{80, 24}, {70, 24}, {40, 24}} {
		card := ansi.Strip(selectedBookCardBorder(book, size[0], size[1]))
		if got, want := len(strings.Split(card, "\n")), selectedBookCardRows(size[0], size[1]); got != want {
			t.Fatalf("card at %v has %d lines, want %d: %q", size, got, want, card)
		}
		for _, value := range []string{"Categories", "2015", "6", "人民邮电出版社", "Chinese", "540", "7115390576", "9787115390578"} {
			if !strings.Contains(card, value) {
				t.Errorf("card at %v omits %q", size, value)
			}
		}
		if strings.Contains(card, "540 pp") || (size[0] >= 78 && !strings.Contains(card, "Pages: 540")) {
			t.Errorf("card at %v formats pages incorrectly", size)
		}
		for _, line := range strings.Split(card, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("card at %v exceeds terminal width", size)
			}
		}
	}
}

func TestSelectedBookDetailLoadsOnlyForCurrentSelection(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"success":1,"book":{"id":2,"hash":"second","title":"Second","categories":"Relationships","pages":540}}`)
	}))
	defer server.Close()
	c := zlib.NewClient(zlib.WithDomain(server.URL))
	c.SetMode(zlib.ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "1", "remix_userkey": "fixture"})
	m := newBookFeedSelectModel("Recommended", nil).withBookDetails(c)
	m.state = searchStateResults
	m.books = []zlib.Book{{ID: "1", Hash: "first", Name: "First"}, {ID: "2", Hash: "second", Name: "Second"}}
	if m.queueSelectedDetail() == nil {
		t.Fatal("first selection did not schedule its detail")
	}
	firstSequence := m.details.sequence
	m.cursor = 1
	if m.queueSelectedDetail() == nil {
		t.Fatal("second selection did not schedule its detail")
	}
	next, staleCmd := m.Update(bookDetailReadyMsg{ref: "1:first", sequence: firstSequence})
	m = next.(searchSelectModel)
	if staleCmd != nil {
		t.Fatal("stale selection started a detail request")
	}
	next, fetch := m.Update(bookDetailReadyMsg{ref: "2:second", sequence: m.details.sequence})
	m = next.(searchSelectModel)
	if fetch == nil {
		t.Fatal("current selection did not fetch its detail")
	}
	next, _ = m.Update(fetch())
	m = next.(searchSelectModel)
	if requests != 1 || m.selectedBook().Categories != "Relationships" || m.selectedBook().Pages != "540" {
		t.Fatalf("selected detail = %+v, requests = %d", m.selectedBook(), requests)
	}
	m.cursor = 0
	if m.selectedBook().Categories != "" {
		t.Fatal("detail from the old selection appeared on another book")
	}
}

func TestSearchEmptyAndErrorOfferRecovery(t *testing.T) {
	m := resultsModel("golang")
	m.width, m.height = 40, 16
	m.books = nil
	m.inShell = true
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Try another title") || !strings.Contains(view, "Ctrl+F search") || !strings.Contains(view, "q back") {
		t.Fatal("empty search has no useful next action")
	}
	m.err = errors.New("request timed out")
	m.retryPage = 2
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Could not load page 2") || !strings.Contains(view, "Retry or change the search") {
		t.Fatal("search error has no recovery path")
	}
	m = step(m, key("/"))
	if m.state != searchStateFilter || m.err == nil || !strings.Contains(ansi.Strip(m.View().Content), "golang") {
		t.Fatal("search error cannot open the query editor")
	}
	m.filter.input.SetValue("rust")
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.state != searchStateLoading || m.err != nil || m.query != "rust" {
		t.Fatal("editing after an error did not restart search")
	}
}

func TestSearchFooterAdvertisesCtrlF(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 16}, {30, 14}} {
		for _, state := range []string{"results", "empty", "error"} {
			m := resultsModel("golang")
			m.width, m.height = size[0], size[1]
			switch state {
			case "empty":
				m.books = nil
			case "error":
				m.err = errors.New("request timed out")
			}
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			footer := strings.ToLower(lines[len(lines)-1])
			if !strings.Contains(footer, "ctrl+f") {
				t.Fatalf("%s footer at %v omits Ctrl+F: %q", state, size, footer)
			}
		}
	}
}

func TestSearchLoadingShowsOnlySpinner(t *testing.T) {
	m := resultsModel("golang")
	m.width, m.height = 80, 24
	m = step(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.loadingPage != 3 || strings.Contains(ansi.Strip(m.View().Content), "Searching page") || strings.Contains(ansi.Strip(m.View().Content), "Page 3") {
		t.Fatal("loading view repeats page information")
	}
	for _, size := range [][2]int{{80, 24}, {40, 16}, {30, 14}} {
		m.width, m.height = size[0], size[1]
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size[1] || strings.Contains(ansi.Strip(m.View().Content), "╭") || !strings.Contains(m.View().Content, m.spinner.View()) {
			t.Fatalf("loading status missing, overflowing, or showing a skeleton at %v", size)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("loading status too wide at %v: %q", size, line)
			}
		}
	}
}

func TestSearchLoadingAndEmptyKeepControlsAtBottom(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 16}, {30, 14}} {
		for _, inShell := range []bool{false, true} {
			m := newSearchSelectModel("golang", nil, nil, false)
			m.width, m.height, m.inShell = size[0], size[1], inShell
			loading := strings.Split(ansi.Strip(m.View().Content), "\n")
			if len(loading) != size[1] || !strings.Contains(loading[len(loading)-1], "esc back") {
				t.Fatalf("loading controls moved at %v, shell=%v", size, inShell)
			}
			m.state, m.loadingPage, m.totalPages = searchStateResults, 0, 3
			empty := strings.Split(ansi.Strip(m.View().Content), "\n")
			searchKey := "Ctrl+F search"
			if len(empty) != size[1] || !strings.Contains(empty[len(empty)-1], searchKey) || !strings.Contains(empty[1], "Page 1 of 3") {
				t.Fatalf("empty search lost page or controls at %v, shell=%v", size, inShell)
			}
		}
	}
}

func TestSearchResultsAppearTogetherAndSelectionStaysStable(t *testing.T) {
	m := newSearchSelectModel("books", nil, nil, false)
	m.width, m.height = 80, 24
	books := make([]zlib.Book, 25)
	for i := range books {
		books[i] = zlib.Book{ID: fmt.Sprint(i + 1), Name: fmt.Sprintf("Book %d", i+1)}
	}
	next, cmd := m.Update(searchResultMsg{books: books, page: 1, totalPages: 2})
	m = next.(searchSelectModel)
	if cmd != nil || len(m.books) != 25 || !strings.Contains(ansi.Strip(m.View().Content), "Book 5") {
		t.Fatal("search results were not ready in the first result frame")
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.cursor != 24 || !strings.Contains(ansi.Strip(m.View().Content), "Book 25") {
		t.Fatal("keyboard navigation did not show the selected result")
	}
}

func TestSearchReducedMotionShowsResultsImmediately(t *testing.T) {
	m := newSearchSelectModel("books", nil, nil, false)
	m.reducedMotion = true
	m.width, m.height = 80, 24
	next, _ := m.Update(searchResultMsg{books: []zlib.Book{{Name: "First"}, {Name: "Second"}}, page: 1, totalPages: 1})
	m = next.(searchSelectModel)
	if !strings.Contains(ansi.Strip(m.View().Content), "Second") {
		t.Fatal("reduced motion hid results")
	}
}

func TestSearchCardKeepsThemeColorsAndDoesNotFlashOnSelection(t *testing.T) {
	previous := currentTheme
	currentTheme = themes["catppuccin-latte"]
	t.Cleanup(func() { currentTheme = previous })
	m := newSearchSelectModel("books", nil, nil, false)
	m.width, m.height = 80, 24
	m.state = searchStateResults
	m.books = []zlib.Book{
		{ID: "1", Name: "First", Authors: []string{"First Author"}},
		{ID: "2", Name: "Second", Authors: []string{"Second Author"}},
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyDown})
	selected := m.View().Content
	author := lipgloss.NewStyle().Foreground(currentTheme.Title).Render("Second Author")
	if !strings.Contains(selected, author) {
		t.Fatal("selected book author does not use the theme's primary text color")
	}
	border := lipgloss.NewStyle().Foreground(currentTheme.Accent).Render("╭")
	if !strings.Contains(selected, strings.TrimSuffix(border, "\x1b[m")) {
		t.Fatal("selected book border does not use the theme accent")
	}
	if got := m.View().Content; got != selected {
		t.Fatal("book card changed between renders without another key press")
	}
}

func feedTestModel() searchSelectModel {
	m := newBookFeedSelectModel("Browse recommended books", nil)
	m.feed = func() ([]zlib.Book, error) { return nil, nil }
	m.state = searchStateResults
	for i := range 100 {
		m.books = append(m.books, zlib.Book{ID: fmt.Sprint(i + 1), Hash: "abcdef", Name: fmt.Sprintf("书籍 %d：中文与 English 长标题", i+1), Authors: []string{"作者 Author"}})
	}
	return m
}

func TestFeedFitsTerminalAndKeepsSelectionVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {40, 16}, {30, 14}, {20, 5}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := feedTestModel()
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(searchSelectModel)
			m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
			view := m.View()
			if !view.AltScreen {
				t.Fatal("feed must use alternate screen")
			}
			lines := strings.Split(view.Content, "\n")
			if len(lines) > size[1] {
				t.Fatalf("%d lines exceed height %d", len(lines), size[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("line exceeds width %d: %q", size[0], line)
				}
			}
			if size[0] >= 30 && !strings.Contains(ansi.Strip(view.Content), "> 100") {
				t.Fatal("last selected book is not visible")
			}
		})
	}
}

func TestFeedNavigationAndDownloadID(t *testing.T) {
	m := feedTestModel()
	m.width, m.height = 80, 24
	m = step(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.cursor != m.feedRows() {
		t.Fatalf("page down cursor = %d", m.cursor)
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.cursor != 0 {
		t.Fatalf("page up cursor = %d", m.cursor)
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if !strings.Contains(m.View().Content, "100:abcdef") {
		t.Fatal("preview lost the complete ID")
	}
	m = step(m, key("enter"))
	m = step(m, tea.KeyPressMsg{Code: tea.KeyRight})
	m = step(m, key("enter"))
	if m.selectedID != "100:abcdef" {
		t.Fatalf("download ID = %q", m.selectedID)
	}
	m = step(m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.cursor != 0 {
		t.Fatal("home did not select the first book")
	}
}

func TestFeedEmptyAndLoadingHaveExitHint(t *testing.T) {
	m := feedTestModel()
	m.books = nil
	for _, state := range []searchState{searchStateLoading, searchStateResults} {
		m.state = state
		if !strings.Contains(m.View().Content, "q quit") {
			t.Fatal("missing exit hint")
		}
	}
}

func TestSearchCardTracksSelectionAndFits(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 16}, {30, 14}} {
		m := feedTestModel()
		m.feed = nil
		m.query = "books"
		m.width, m.height = size[0], size[1]
		m.books[0].Authors = []string{"First author"}
		m.books[1].Authors = []string{"Second author"}
		m.books[1].Extension = "epub"
		before := ansi.Strip(m.View().Content)
		m = step(m, tea.KeyPressMsg{Code: tea.KeyDown})
		after := ansi.Strip(m.View().Content)
		cardEnd := strings.Index(after, "╰")
		if cardEnd < 0 || !strings.Contains(after[:cardEnd], "Second author") || !strings.Contains(after[:cardEnd], "EPUB") || strings.Contains(after[:cardEnd], "First author") {
			t.Fatal("card did not track selection")
		}
		if strings.Count(before[:strings.Index(before, "╰")], "\n") != strings.Count(after[:cardEnd], "\n") {
			t.Fatal("card height changed on selection")
		}
		if len(strings.Split(after, "\n")) > size[1] {
			t.Fatal("search exceeds terminal height")
		}
		for _, line := range strings.Split(after, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("search exceeds terminal width: %q", line)
			}
		}
	}
}

func TestSavedCardAndColumnSpacing(t *testing.T) {
	for _, width := range []int{30, 40, 80, 120} {
		m := newSavedBooksSelectModel(func(int) (zlib.SearchResult, error) { return zlib.SearchResult{}, nil })
		m.state = searchStateResults
		m.width, m.height = width, 24
		m.books = []zlib.Book{
			{ID: "1", Name: "Short title", Authors: []string{"First author"}},
			{ID: "2", Name: "中文书名", Authors: []string{"Second author"}},
		}
		m = step(m, tea.KeyPressMsg{Code: tea.KeyDown})
		view := ansi.Strip(m.View().Content)
		end := strings.Index(view, "╰")
		if end < 0 || !strings.Contains(view[:end], "Second author") || strings.Contains(view[:end], "First author") {
			t.Fatal("saved selection did not update card")
		}
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatal("saved view exceeds terminal height")
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("saved view exceeds terminal width")
			}
			if width >= 70 && strings.Contains(line, "Title") && strings.Contains(line, "Author") {
				if !strings.Contains(line, "#   Title") || strings.Index(line, "Author") != 20 {
					t.Fatal("short titles leave excessive column spacing")
				}
			}
		}
	}
}

func TestPopularCardTracksSelection(t *testing.T) {
	m := feedTestModel()
	m.feedTitle = "Browse the most popular books"
	m.width, m.height = 80, 24
	m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	view := ansi.Strip(m.View().Content)
	end := strings.Index(view, "╰")
	if end < 0 || !strings.Contains(view[:end], "100:abcdef") {
		t.Fatal("popular card does not show selected book")
	}
	if len(strings.Split(view, "\n")) > m.height || !strings.Contains(view, "> 100") {
		t.Fatal("popular list overflowed or hid selection")
	}
}

func TestRecommendedCardTracksSelection(t *testing.T) {
	m := feedTestModel()
	m.width, m.height = 80, 24
	m.books[0].Authors = []string{"First author"}
	m.books[1].Authors = []string{"Second author"}
	m.books[1].Extension = "epub"
	before := ansi.Strip(m.View().Content)
	m = step(m, tea.KeyPressMsg{Code: tea.KeyDown})
	after := ansi.Strip(m.View().Content)
	cardEnd := strings.Index(after, "╰")
	if cardEnd < 0 || !strings.Contains(after[:cardEnd], "Second author") || !strings.Contains(after[:cardEnd], "EPUB") || strings.Contains(after[:cardEnd], "First author") {
		t.Fatal("recommended card did not track selection")
	}
	if strings.Count(before[:strings.Index(before, "╰")], "\n") != strings.Count(after[:cardEnd], "\n") {
		t.Fatal("recommended card height changed on selection")
	}
}

func TestBookColumnsUseAvailableWidth(t *testing.T) {
	for _, needs := range [][2]int{{5, 40}, {80, 8}, {30, 35}, {120, 100}} {
		for width := 30; width <= 240; width++ {
			title, detail, gap := bookColumnWidths(width, 3, needs[0], needs[1])
			used := 2 + 3 + gap + title
			if detail > 0 {
				used += gap + detail
			}
			if used > width || title <= 0 || title > needs[0] || detail > needs[1] {
				t.Fatalf("invalid column widths at %d: %d/%d", width, title, detail)
			}
			if 2+3+4+needs[0]+needs[1] <= width && (title != needs[0] || detail != needs[1]) {
				t.Fatalf("content truncated despite sufficient space at %d", width)
			}
			if detail > 0 && (title < needs[0] || detail < needs[1]) && used != width {
				t.Fatalf("unused space while truncating at %d", width)
			}
		}
	}
}
