package cli

import (
	"errors"
	"os"
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
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Send to Kindle?") || !strings.Contains(view, "Send + download") || strings.Contains(view, "Cancel") || m.selectedID != "" {
		t.Fatal("Kindle shortcut must ask before downloading")
	}
	m = step(m, key("enter"))
	if m.selectedID != "1:abc" || !m.sendToKindle || !m.sendOnly {
		t.Fatal("default confirmation must send without keeping a download")
	}
	m.selectedID = ""
	m = step(m, key("ctrl+k"))
	m = step(m, key("esc"))
	m = step(m, key("enter"))
	if m.sendToKindle || m.sendOnly || !strings.Contains(ansi.Strip(m.View().Content), "Download book?") {
		t.Fatal("cancelled Kindle action leaked into a normal download")
	}
	m = step(m, key("esc"))
	m = step(m, key("ctrl+k"))
	m = step(m, key("right"))
	next, cmd := m.Update(key("enter"))
	m = next.(searchSelectModel)
	if m.selectedID != "1:abc" || !m.sendToKindle || m.sendOnly || cmd == nil {
		t.Fatal("confirmed Kindle action did not select the book for delivery")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("confirmed action did not leave the result selector")
	}
}

func TestShellSearchKindleSendOnly(t *testing.T) {
	for _, outcome := range []string{"sent", "cancelled", "failed", "unsupported"} {
		t.Run(outcome, func(t *testing.T) {
			isolateConfigHome(t)
			t.Setenv(kindleSMTPPasswordEnv, "fixture-password")
			if err := config.SaveKindleConfig(zlib.KindleConfig{To: "reader@kindle.com", From: "sender@example.com", SMTPHost: "smtp.example.com", SMTPPort: 587}); err != nil {
				t.Fatal(err)
			}
			m := newShellModel()
			list := resultsModel("books")
			list.cursor = 0
			list.client = zlib.NewClient()
			list.books = []zlib.Book{{ID: "1", Hash: "abc", Name: "Book", Extension: "epub"}}
			m.open("list", list)
			m, _ = shellStep(m, key("ctrl+k"))
			m, cmd := shellStep(m, key("enter"))
			m, _ = shellStep(m, cmd()) // Do not execute the network download.
			if m.kind != "download" || !m.download.sendOnly || !m.download.sendOnComplete || m.download.dir == "" {
				t.Fatal("send-only must skip the directory picker and download temporarily")
			}
			dir := m.download.dir
			t.Cleanup(func() { os.RemoveAll(dir) })
			if outcome == "cancelled" {
				page := m.page.(downloadModel)
				page.state = stateCancelled
				m.page = page
				m.completed()
			} else {
				ext := ".epub"
				if outcome == "unsupported" {
					ext = ".zip"
				}
				path := filepath.Join(dir, "book"+ext)
				if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
				m, cmd = shellStep(m, downloadDoneMsg{path: path, size: 7})
				m, _ = shellStep(m, cmd())
				if outcome != "unsupported" {
					if m.kind != "kindle-send" || m.page.(kindleSendModel).filePath != path {
						t.Fatal("temporary download did not advance to Kindle delivery")
					}
					if outcome == "failed" {
						m, cmd = shellStep(m, errMsg{err: errors.New("fixture send failure")})
						m, _ = shellStep(m, cmd())
						if m.kind != "kindle-error" {
							t.Fatal("delivery failure must show the error")
						}
						m.completed()
					} else {
						m, cmd = shellStep(m, kindleSendDoneMsg{})
						m, _ = shellStep(m, cmd())
					}
				}
			}
			if m.kind != "list" || m.page.(searchSelectModel).sendOnly || m.page.(searchSelectModel).sendToKindle {
				t.Fatal("send-only must return to search without retaining the action")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("temporary download directory was not removed: %v", err)
			}
		})
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
