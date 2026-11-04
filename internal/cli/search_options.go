package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/heartleo/zlib/internal/zlib"
)

type searchFilterValues struct {
	Extensions []string
	Languages  []string
	FromYear   int
	ToYear     int
	Exact      bool
	Order      string
}

func defaultSearchOptions() *zlib.SearchOptions {
	opts, _ := searchOptions(searchFilterValues{
		Extensions: []string{os.Getenv(defaultFormatEnvVar)},
		Languages:  strings.Split(os.Getenv(defaultLanguagesEnvVar), ","),
	})
	return opts
}

func searchOptions(values searchFilterValues) (*zlib.SearchOptions, error) {
	from, to := values.FromYear, values.ToYear
	if from < 0 || to < 0 || from > 0 && from < 1000 || to > 0 && to < 1000 || from > 9999 || to > 9999 || from > 0 && to > 0 && from > to {
		return nil, fmt.Errorf("invalid year range: years must be between 1000 and 9999, with the earliest year first")
	}
	order := zlib.OrderOption(strings.TrimSpace(values.Order))
	if order != "" {
		valid := false
		for _, choice := range searchSorts {
			if choice.value == order {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("invalid search order %q", values.Order)
		}
	}
	opts := &zlib.SearchOptions{FromYear: from, ToYear: to, Exact: values.Exact, Order: order}
	seen := make(map[zlib.Extension]bool)
	for _, raw := range values.Extensions {
		ext := zlib.Extension(strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(raw), ".")))
		if ext == "" || ext == "ANY" || seen[ext] {
			continue
		}
		seen[ext] = true
		opts.Extensions = append(opts.Extensions, ext)
	}
	seenLanguages := make(map[string]bool)
	for _, raw := range values.Languages {
		if language := strings.ToLower(strings.TrimSpace(raw)); language != "" && language != "any" && !seenLanguages[language] {
			seenLanguages[language] = true
			opts.Languages = append(opts.Languages, zlib.Language(language))
		}
	}
	if len(opts.Extensions) == 0 && len(opts.Languages) == 0 && from == 0 && to == 0 && !values.Exact && order == "" {
		return nil, nil
	}
	return opts, nil
}
