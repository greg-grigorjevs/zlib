package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	zlib "github.com/heartleo/zlib/internal/zlib"
)

func TestInteractiveSearchUsesFiftyBooksPerPage(t *testing.T) {
	request := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eapi/book/search" || r.ParseForm() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		request <- r.Form.Get("page") + ":" + r.Form.Get("limit")
		fmt.Fprint(w, `{"success":1,"books":[],"pagination":{"current":2,"total_pages":3}}`)
	}))
	defer server.Close()
	client := zlib.NewClient(zlib.WithDomain(server.URL))
	client.SetCookies(map[string]string{"remix_userid": "1", "remix_userkey": "fixture"})
	m := newSearchSelectModel("books", client, nil, false)
	if _, ok := m.fetchPage(2)().(searchResultMsg); !ok {
		t.Fatal("interactive search did not return an EAPI result")
	}
	if got := <-request; got != "2:50" {
		t.Fatalf("interactive search requested page:limit %q, want 2:50", got)
	}
}

func TestSearchViewportAndInlineEditor(t *testing.T) {
	for _, width := range []int{30, 40, 80, 120} {
		m := resultsModel("中文 search")
		m.books = nil
		for i := range 50 {
			m.books = append(m.books, zlib.Book{ID: fmt.Sprint(i + 1), Hash: "abcdef", Name: "中文书名 with a long English title", Authors: []string{"Author"}})
		}
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = next.(searchSelectModel)
		m = step(m, tea.KeyPressMsg{Code: tea.KeyEnd})
		if m.cursor != 49 || !strings.Contains(ansi.Strip(m.View().Content), "> 50") {
			t.Fatal("last search result is not visible")
		}
		for _, editing := range []bool{false, true} {
			if editing {
				m = step(m, key("/"))
			}
			view := m.View()
			if !view.AltScreen {
				t.Fatal("search needs alternate screen")
			}
			lines := strings.Split(view.Content, "\n")
			if len(lines) > 24 {
				t.Fatal("search exceeds terminal height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > width {
					t.Fatalf("line exceeds %d: %q", width, line)
				}
			}
			if editing && !strings.Contains(ansi.Strip(view.Content), "search") {
				t.Fatalf("missing editor help: %q", ansi.Strip(view.Content))
			}
		}
	}
}

func TestCtrlFReopensDirectSearch(t *testing.T) {
	for _, state := range []string{"results", "empty", "error"} {
		t.Run(state, func(t *testing.T) {
			m := resultsModel("golang")
			switch state {
			case "empty":
				m.books = nil
			case "error":
				m.err = fmt.Errorf("search failed")
			}
			m = step(m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
			if m.state != searchStateFilter || m.filter.input.Value() != "golang" {
				t.Fatal("Ctrl+F did not reopen the current query")
			}
			m.filter.input.SetValue("new query")
			m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			if m.query != "new query" || m.state != searchStateLoading {
				t.Fatal("refined search was not submitted")
			}
		})
	}
}

func TestEmptySearchCanEditQuery(t *testing.T) {
	m := resultsModel("no results")
	m.books = nil
	m = step(m, key("/"))
	if !strings.Contains(ansi.Strip(m.View().Content), "search") {
		t.Fatalf("empty editor has no submit hint: %q", ansi.Strip(m.View().Content))
	}
	m.filter.input.SetValue("new query")
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.query != "new query" || m.state != searchStateLoading {
		t.Fatal("empty search cannot be retried")
	}
}

func TestBookFeedModelHasNoSearchOrPagination(t *testing.T) {
	m := newBookFeedSelectModel("Recommended books", func() ([]zlib.Book, error) {
		return []zlib.Book{{ID: "42", Hash: "abc", Name: "A Book"}}, nil
	})
	message := m.fetchPage(1)()
	result, ok := message.(searchResultMsg)
	if !ok || len(result.books) != 1 || result.books[0].ID != "42" {
		t.Fatalf("feed message = %+v", message)
	}
	m.state = searchStateResults
	m.books = result.books
	m.totalPages = 1
	m = step(m, key("/"))
	if m.state != searchStateResults {
		t.Fatalf("slash changed feed state to %v", m.state)
	}
	m = step(m, key("l"))
	if m.state != searchStateResults {
		t.Fatalf("right changed feed state to %v", m.state)
	}
	view := m.View().Content
	if !strings.Contains(view, "Recommended books") || strings.Contains(view, "page") || strings.Contains(view, " search") {
		t.Fatalf("unexpected feed view: %q", view)
	}
	m = step(m, key("enter"))
	m = step(m, tea.KeyPressMsg{Code: tea.KeyRight})
	m = step(m, key("enter"))
	if m.selectedID != "42:abc" {
		t.Fatalf("selectedID = %q", m.selectedID)
	}
}

func TestSavedBooksModelPagesWithoutSearch(t *testing.T) {
	m := newSavedBooksSelectModel(func(page int) (zlib.SearchResult, error) {
		return zlib.SearchResult{Books: []zlib.Book{{ID: "42", Hash: "abc", Name: "Saved"}}, Page: page, TotalPages: 3}, nil
	})
	m.state = searchStateResults
	m.page = 1
	m.totalPages = 3
	m.books = []zlib.Book{{ID: "42", Hash: "abc", Name: "Saved"}}
	m = step(m, key("/"))
	if m.state != searchStateResults {
		t.Fatalf("slash changed saved state to %v", m.state)
	}
	m = step(m, key("l"))
	if m.state != searchStateLoading {
		t.Fatalf("right did not load the next page: %v", m.state)
	}
	if !strings.Contains(m.View().Content, "Favorites") {
		t.Fatal("saved title missing")
	}
}

func resultsModel(query string) searchSelectModel {
	m := newSearchSelectModel(query, nil, nil, false)
	m.state = searchStateResults
	m.books = []zlib.Book{{Name: "A"}, {Name: "B"}}
	m.totalPages = 3
	m.page = 2
	m.cursor = 1
	return m
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func step(m searchSelectModel, k tea.KeyPressMsg) searchSelectModel {
	next, _ := m.Update(k)
	return next.(searchSelectModel)
}

func TestSlashEntersFilterPrefilled(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	if m.state != searchStateFilter {
		t.Fatalf("state = %v, want filter", m.state)
	}
	if m.filter.input.Value() != "golang" {
		t.Errorf("input = %q, want prefilled %q", m.filter.input.Value(), "golang")
	}
}

func TestFilterEnterRerunsWithNewQuery(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m.filter.input.SetValue("rust")
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.state != searchStateLoading {
		t.Fatalf("state = %v, want loading", m.state)
	}
	if m.query != "rust" {
		t.Errorf("query = %q, want rust", m.query)
	}
	if m.page != 1 {
		t.Errorf("page = %d, want reset to 1", m.page)
	}
	if m.totalPages != 0 {
		t.Errorf("totalPages = %d, want reset to 0 while loading", m.totalPages)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want reset to 0", m.cursor)
	}
}

func TestFilterEnterPreservesNonEmptyWhitespace(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m.filter.input.SetValue(" golang ")
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.state != searchStateLoading {
		t.Fatalf("state = %v, want loading", m.state)
	}
	if m.query != " golang " {
		t.Errorf("query = %q, want whitespace preserved", m.query)
	}
}

func TestFilterEnterUnchangedQueryReturnsToResults(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m = step(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.state != searchStateLoading {
		t.Fatalf("state = %v, want loading", m.state)
	}
	if m.query != "golang" {
		t.Errorf("query = %q, want unchanged golang", m.query)
	}
}

func TestFilterEscCancels(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m.filter.input.SetValue("rust")
	m = step(m, key("esc"))
	if m.state != searchStateResults {
		t.Fatalf("state = %v, want results", m.state)
	}
	if m.query != "golang" {
		t.Errorf("query = %q, want unchanged golang after cancel", m.query)
	}
}

func TestFilterCtrlCQuits(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m = step(m, key("ctrl+c"))
	if !m.quitting {
		t.Errorf("ctrl+c while filtering should quit")
	}
	if m.state != searchStateFilter {
		t.Errorf("state = %v, want still filter while quitting", m.state)
	}
}

func TestFilterTypingDoesNotQuitOnQ(t *testing.T) {
	m := step(resultsModel("golang"), key("/"))
	m.filter.input.SetValue("")
	m = step(m, key("q")) // must be treated as text, not quit
	if m.quitting {
		t.Errorf("typing 'q' while filtering should not quit")
	}
	if m.state != searchStateFilter {
		t.Errorf("state = %v, want still filter", m.state)
	}
}
