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

func TestDefaultFormatSearchOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, saved, flag, value string
		want                     []zlib.Extension
	}{
		{name: "unset"},
		{name: "saved default", saved: "epub", want: []zlib.Extension{zlib.ExtEPUB}},
		{name: "format override", saved: "epub", flag: "format", value: "pdf", want: []zlib.Extension{zlib.ExtPDF}},
		{name: "ext override", saved: "epub", flag: "ext", value: "pdf", want: []zlib.Extension{zlib.ExtPDF}},
		{name: "any override", saved: "epub", flag: "format", value: "any"},
		{name: "empty override", saved: "epub", flag: "format", value: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(defaultFormatEnvVar, tc.saved)
			cmd := &cobra.Command{}
			cmd.Flags().StringArray("ext", nil, "")
			cmd.Flags().StringArray("format", nil, "")
			if tc.flag != "" {
				if err := cmd.Flags().Set(tc.flag, tc.value); err != nil {
					t.Fatal(err)
				}
			}
			opts, err := searchOptsFromFlags(cmd)
			if err != nil {
				t.Fatal(err)
			}
			var got []zlib.Extension
			if opts != nil {
				got = opts.Extensions
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("extensions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultFormatFormCanClearAndRefine(t *testing.T) {
	t.Setenv(defaultFormatEnvVar, "epub")
	form := newSearchFilterForm("Dune", defaultSearchOptions())
	if !slices.Equal(form.extensions, []zlib.Extension{zlib.ExtEPUB}) {
		t.Fatalf("form extensions = %v", form.extensions)
	}
	form.extensions = nil
	refined := newSearchFilterForm(form.query, form.searchOptions())
	if len(refined.extensions) != 0 {
		t.Fatal("refining restored a default the user cleared")
	}
}

func TestDefaultFormatShellSearchRequest(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv(defaultFormatEnvVar, "EPUB")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if got := r.Form.Get("extensions[]"); got != "epub" {
			t.Errorf("request format = %q, want epub", got)
		}
		fmt.Fprint(w, `{"success":1,"books":[],"pagination":{"current":1,"total_pages":1}}`)
	}))
	defer server.Close()
	client := zlib.NewClient(zlib.WithDomain(server.URL))
	client.SetCookies(map[string]string{"remix_userid": "1", "remix_userkey": "k"})
	previous := loadClient
	loadClient = func() (*zlib.Client, error) { return client, nil }
	t.Cleanup(func() { loadClient = previous })
	m := newShellModel()
	m.dispatch("/search")
	form, ok := m.page.(searchFilterForm)
	if !ok || !slices.Equal(form.extensions, []zlib.Extension{zlib.ExtEPUB}) {
		t.Fatal("shell search form did not preselect default format")
	}
	m.dispatch("Dune")
	list, ok := m.page.(searchSelectModel)
	if !ok {
		t.Fatal("query did not open search results")
	}
	if _, ok := list.fetchPage(1)().(searchResultMsg); !ok {
		t.Fatal("search request failed")
	}
}

func TestDefaultFormatMCPOverrides(t *testing.T) {
	t.Setenv(defaultFormatEnvVar, "epub")
	for _, tc := range []struct {
		exts []string
		want []zlib.Extension
	}{
		{nil, []zlib.Extension{zlib.ExtEPUB}},
		{[]string{"pdf"}, []zlib.Extension{zlib.ExtPDF}},
		{[]string{"any"}, nil},
	} {
		opts, err := searchOptsFromMCPInput(mcpSearchBooksInput{Extensions: tc.exts})
		if err != nil {
			t.Fatal(err)
		}
		var got []zlib.Extension
		if opts != nil {
			got = opts.Extensions
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("extensions = %v, want %v", got, tc.want)
		}
	}
}

func TestDefaultFormatSettingSaveReloadAndReset(t *testing.T) {
	t.Setenv(defaultFormatEnvVar, "")
	s := testSettingsStore(t)
	m, err := newSettingsModel(t.Context(), s, "search")
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(key("enter"))
	m = next.(settingsModel)
	if m.key != defaultFormatEnvVar {
		t.Fatal("default format row did not open its editor")
	}
	m.input.SetValue(" .epub ")
	next, _ = m.Update(key("enter"))
	m = next.(settingsModel)
	if m.page != "search" || os.Getenv(defaultFormatEnvVar) != "EPUB" {
		t.Fatalf("save did not activate default: page=%s, value=%s", m.page, os.Getenv(defaultFormatEnvVar))
	}
	loaded, err := newSettingsModel(t.Context(), s, "search")
	if err != nil || loaded.value(defaultFormatEnvVar).Saved != "EPUB" {
		t.Fatalf("saved format did not reload: %v", err)
	}
	m.edit(defaultFormatEnvVar)
	m.input.SetValue("not-a-format")
	next, _ = m.Update(key("enter"))
	m = next.(settingsModel)
	if m.page != "edit" || m.value(defaultFormatEnvVar).Saved != "EPUB" {
		t.Fatal("invalid format changed the saved default")
	}
	next, _ = m.Update(key("ctrl+r"))
	next, _ = next.Update(key("enter"))
	m = next.(settingsModel)
	if m.value(defaultFormatEnvVar).Source != "Default" || defaultSearchOptions() != nil {
		t.Fatal("reset did not restore all formats")
	}
}

func TestDefaultFormatCommandPersists(t *testing.T) {
	isolateConfigHome(t)
	t.Chdir(t.TempDir())
	for _, value := range []string{"epub", "any"} {
		if _, err := executeRootForTest(t, "config", "search", "format", value); err != nil {
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
		want := "EPUB"
		if value == "any" {
			want = ""
		}
		if values[defaultFormatEnvVar] != want {
			t.Fatalf("saved format = %q, want %q", values[defaultFormatEnvVar], want)
		}
	}
	if _, err := executeRootForTest(t, "config", "search", "format", "invalid"); err == nil {
		t.Fatal("invalid default format accepted")
	}
}
