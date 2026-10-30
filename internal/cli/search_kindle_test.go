package cli

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/heartleo/zlib/internal/config"
	"github.com/heartleo/zlib/internal/zlib"
)

func TestSearchKindleConfirmation(t *testing.T) {
	m := resultsModel("books")
	m.cursor = 0
	m.books = []zlib.Book{{ID: "1", Hash: "abc", Name: "Book", Extension: "epub"}}
	if !strings.Contains(ansi.Strip(m.View().Content), "Ctrl+K Kindle") {
		t.Fatal("search results do not advertise Kindle delivery")
	}
	m = step(m, key("ctrl+k"))
	if !strings.Contains(ansi.Strip(m.View().Content), "Download and send to Kindle?") || m.selectedID != "" {
		t.Fatal("Kindle shortcut must ask before downloading")
	}
	m = step(m, key("enter"))
	if m.selectedID != "" || m.sendToKindle {
		t.Fatal("default confirmation must cancel Kindle delivery")
	}
	m = step(m, key("ctrl+k"))
	m = step(m, key("esc"))
	m = step(m, key("enter"))
	if m.sendToKindle || !strings.Contains(ansi.Strip(m.View().Content), "Download book?") {
		t.Fatal("cancelled Kindle action leaked into a normal download")
	}
	m = step(m, key("esc"))
	m = step(m, key("ctrl+k"))
	m = step(m, key("right"))
	next, cmd := m.Update(key("enter"))
	m = next.(searchSelectModel)
	if m.selectedID != "1:abc" || !m.sendToKindle || cmd == nil {
		t.Fatal("confirmed Kindle action did not select the book for delivery")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmed action did not leave the result selector")
	}
}

func TestSearchKindleRequiresSelectableResult(t *testing.T) {
	for _, books := range [][]zlib.Book{nil, {{Name: "No download reference"}}} {
		m := newSearchSelectModel("books", nil, nil, false)
		m.state = searchStateResults
		m.books = books
		m = step(m, key("ctrl+k"))
		if m.confirmDownload || m.sendToKindle || m.selectedID != "" {
			t.Fatal("Kindle shortcut accepted an undownloadable result")
		}
	}
	m := newSearchSelectModel("books", nil, nil, false)
	m = step(m, key("ctrl+k"))
	if m.confirmDownload || m.sendToKindle {
		t.Fatal("Kindle shortcut was active while loading")
	}
}

func TestShellSearchKindleDownloadsThenSends(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv(kindleSMTPPasswordEnv, "fixture-password")
	if err := config.SaveKindleConfig(zlib.KindleConfig{To: "reader@kindle.com", From: "sender@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587}); err != nil {
		t.Fatal(err)
	}
	m := newShellModel()
	list := newSearchSelectModel("books", zlib.NewClient(), nil, false)
	list.state = searchStateResults
	list.books = []zlib.Book{{ID: "1", Hash: "abc", Name: "Book", Extension: "epub"}}
	m.open("list", list)
	m, _ = shellStep(m, key("ctrl+k"))
	m, _ = shellStep(m, key("right"))
	m, cmd := shellStep(m, key("enter"))
	if cmd == nil {
		t.Fatal("confirmation did not finish the selector")
	}
	m, _ = shellStep(m, cmd())
	if m.kind != "directory" {
		t.Fatalf("expected directory selection, got %q", m.kind)
	}
	directory := m.page.(downloadDirectoryModel)
	directory.accepted = true
	directory.dir = t.TempDir()
	m.page = directory
	m.completed() // Do not execute the real network download command.
	path := filepath.Join(directory.dir, "book.epub")
	m, cmd = shellStep(m, downloadDoneMsg{path: path, size: 10})
	if cmd == nil {
		t.Fatal("download completion did not advance to Kindle delivery")
	}
	m, _ = shellStep(m, cmd())
	if m.kind != "kindle-send" || m.page.(kindleSendModel).filePath != path {
		t.Fatalf("expected Kindle send for downloaded file, got %q", m.kind)
	}
	if m.previous.(searchSelectModel).sendToKindle {
		t.Fatal("returning to the search would retain the previous Kindle action")
	}
	sent := m.page.(kindleSendModel)
	sent.done = true
	m.page = sent
	m.completed()
	if m.kind != "download" || m.page.(downloadModel).sendOnComplete {
		t.Fatal("delivery must return to completion without sending again")
	}
}
