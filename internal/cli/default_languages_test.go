package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	zlib "github.com/heartleo/zlib/internal/zlib"
	"github.com/spf13/cobra"
)

func TestDefaultLanguagesOverrides(t *testing.T) {
	t.Setenv(defaultFormatEnvVar, "")
	t.Setenv(defaultLanguagesEnvVar, "english,russian")
	for _, tc := range []struct {
		name   string
		values []string
		want   []zlib.Language
	}{
		{"saved", nil, []zlib.Language{zlib.LangEnglish, zlib.LangRussian}},
		{"override", []string{" French ", "french"}, []zlib.Language{zlib.LangFrench}},
		{"multiple", []string{"english", "russian"}, []zlib.Language{zlib.LangEnglish, zlib.LangRussian}},
		{"any", []string{"any"}, nil},
		{"empty", []string{""}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().StringArray("language", nil, "")
			for _, value := range tc.values {
				if err := cmd.Flags().Set("language", value); err != nil {
					t.Fatal(err)
				}
			}
			cliOpts, err := searchOptsFromFlags(cmd)
			if err != nil {
				t.Fatal(err)
			}
			mcpOpts, err := searchOptsFromMCPInput(mcpSearchBooksInput{Languages: tc.values})
			if err != nil {
				t.Fatal(err)
			}
			for _, opts := range []*zlib.SearchOptions{cliOpts, mcpOpts} {
				var got []zlib.Language
				if opts != nil {
					got = opts.Languages
				}
				if !slices.Equal(got, tc.want) {
					t.Fatalf("languages = %v, want %v", got, tc.want)
				}
			}
		})
	}
	form := newSearchFilterForm("books", defaultSearchOptions())
	if !slices.Equal(form.languages, []zlib.Language{zlib.LangEnglish, zlib.LangRussian}) {
		t.Fatal("form did not preselect default languages")
	}
	form.languages = nil
	if len(newSearchFilterForm(form.query, form.searchOptions()).languages) != 0 {
		t.Fatal("refine restored cleared languages")
	}
}

func TestDefaultLanguagesSettingSaveReloadAndReset(t *testing.T) {
	t.Setenv(defaultLanguagesEnvVar, "")
	s := testSettingsStore(t)
	m, err := newSettingsModel(t.Context(), s, "search")
	if err != nil {
		t.Fatal(err)
	}
	m.cursor = 1
	next, _ := m.Update(key("enter"))
	m = next.(settingsModel)
	if m.key != defaultLanguagesEnvVar {
		t.Fatal("languages row did not open editor")
	}
	m.input.SetValue(" English, Russian, english ")
	next, _ = m.Update(key("enter"))
	m = next.(settingsModel)
	if m.page != "search" || os.Getenv(defaultLanguagesEnvVar) != "english,russian" {
		t.Fatal("save did not activate languages")
	}
	loaded, err := newSettingsModel(t.Context(), s, "home")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.value(defaultLanguagesEnvVar).Saved != "english,russian" {
		t.Fatal("languages did not reload")
	}
	loaded.cursor = 5
	next, _ = loaded.Update(key("enter"))
	if next.(settingsModel).key != defaultLanguagesEnvVar {
		t.Fatal("home row opened wrong editor")
	}
	m.edit(defaultLanguagesEnvVar)
	m.input.SetValue("not-a-language")
	next, _ = m.Update(key("enter"))
	m = next.(settingsModel)
	if m.page != "edit" || m.value(defaultLanguagesEnvVar).Saved != "english,russian" {
		t.Fatal("invalid language changed saved setting")
	}
	next, _ = m.Update(key("ctrl+r"))
	next, _ = next.Update(key("enter"))
	m = next.(settingsModel)
	if m.value(defaultLanguagesEnvVar).Source != "Default" || os.Getenv(defaultLanguagesEnvVar) != "" {
		t.Fatal("reset failed")
	}
}

func TestDefaultLanguagesCommandPersists(t *testing.T) {
	isolateConfigHome(t)
	t.Chdir(t.TempDir())
	for _, args := range [][]string{{"English", "Russian"}, {"english,russian"}, {"any"}} {
		if _, err := executeRootForTest(t, append([]string{"config", "search", "languages"}, args...)...); err != nil {
			t.Fatal(err)
		}
		path, err := globalDotEnvPath()
		if err != nil {
			t.Fatal(err)
		}
		values, err := readSettingsEnv(path)
		if err != nil {
			t.Fatal(err)
		}
		want := "english,russian"
		if args[0] == "any" {
			want = ""
		}
		if values[defaultLanguagesEnvVar] != want {
			t.Fatalf("saved languages = %q, want %q", values[defaultLanguagesEnvVar], want)
		}
	}
	if _, err := executeRootForTest(t, "config", "search", "languages", "invalid"); err == nil {
		t.Fatal("invalid language accepted")
	}
}

func TestDefaultLanguagesShellSearchRequest(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv(defaultLanguagesEnvVar, "english,russian")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if !slices.Equal(r.Form["languages[]"], []string{"english", "russian"}) {
			t.Errorf("request languages = %v", r.Form["languages[]"])
		}
		fmt.Fprint(w, `{"success":1,"books":[],"pagination":{"current":1,"total_pages":1}}`)
	}))
	defer server.Close()
	client := zlib.NewClient(zlib.WithDomain(server.URL))
	client.SetCookies(map[string]string{"remix_userid": "1", "remix_userkey": "FAKE-KEY"})
	previous := loadClient
	loadClient = func() (*zlib.Client, error) { return client, nil }
	t.Cleanup(func() { loadClient = previous })
	m := newShellModel()
	m.dispatch("books")
	list, ok := m.page.(searchSelectModel)
	if !ok {
		t.Fatal("query did not open results")
	}
	if _, ok := list.fetchPage(1)().(searchResultMsg); !ok {
		t.Fatal("search request failed")
	}
}
