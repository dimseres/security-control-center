package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	reDataI18n            = regexp.MustCompile(`data-i18n="([^"]+)"`)
	reDataI18nPlaceholder = regexp.MustCompile(`data-i18n-placeholder="([^"]+)"`)
	reBerkutT             = regexp.MustCompile(`BerkutI18n\.t\(\s*['"]([^'"]+)['"]\s*\)`)
	reLocalT              = regexp.MustCompile(`\bt\(\s*['"]([^'"]+)['"]\s*\)`)
	reNamedPlaceholder    = regexp.MustCompile(`\{[a-zA-Z0-9_.-]+\}`)
	rePrintfPlaceholder   = regexp.MustCompile(`%(\[[0-9]+\])?[-+# 0]*[0-9]*(\.[0-9]+)?[bcdefFgGosqTtUvVxX%]`)
)

func TestI18NKeyCoverage(t *testing.T) {
	ru := mustLoadLang(t, filepath.Join("..", "gui", "static", "i18n", "ru.json"))
	en := mustLoadLang(t, filepath.Join("..", "gui", "static", "i18n", "en.json"))

	missingInEN := diffKeys(ru, en)
	missingInRU := diffKeys(en, ru)
	if len(missingInEN) > 0 || len(missingInRU) > 0 {
		t.Fatalf("language key mismatch: missing in en=%v missing in ru=%v", sample(missingInEN), sample(missingInRU))
	}

	used := collectUsedI18NKeys(t, filepath.Join("..", "gui", "static"))
	var missingUsed []string
	for _, key := range used {
		if _, ok := ru[key]; !ok {
			missingUsed = append(missingUsed, key+" (ru)")
		}
		if _, ok := en[key]; !ok {
			missingUsed = append(missingUsed, key+" (en)")
		}
	}
	if len(missingUsed) > 0 {
		sort.Strings(missingUsed)
		t.Fatalf("missing i18n keys for used ui references: %v", sample(missingUsed))
	}
}

func TestI18NValuesNonEmptyAndPlaceholdersMatch(t *testing.T) {
	ru := mustLoadLang(t, filepath.Join("..", "gui", "static", "i18n", "ru.json"))
	en := mustLoadLang(t, filepath.Join("..", "gui", "static", "i18n", "en.json"))

	var emptyValues []string
	var placeholderMismatch []string

	for key, ruVal := range ru {
		enVal, ok := en[key]
		if !ok {
			continue
		}
		if strings.TrimSpace(ruVal) == "" {
			emptyValues = append(emptyValues, key+" (ru)")
		}
		if strings.TrimSpace(enVal) == "" {
			emptyValues = append(emptyValues, key+" (en)")
		}

		ruPH := placeholdersSet(ruVal)
		enPH := placeholdersSet(enVal)
		if !sameStringSet(ruPH, enPH) {
			placeholderMismatch = append(placeholderMismatch, key)
		}
	}

	if len(emptyValues) > 0 {
		sort.Strings(emptyValues)
		t.Fatalf("empty i18n values: %v", sample(emptyValues))
	}
	if len(placeholderMismatch) > 0 {
		sort.Strings(placeholderMismatch)
		t.Fatalf("placeholder mismatch between ru/en: %v", sample(placeholderMismatch))
	}
}

func mustLoadLang(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return out
}

func collectUsedI18NKeys(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]struct{}{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.Contains(path, string(filepath.Separator)+"vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".js" {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		content := string(raw)
		addMatches(content, reDataI18n, seen)
		addMatches(content, reDataI18nPlaceholder, seen)
		addMatches(content, reBerkutT, seen)
		addMatches(content, reLocalT, seen)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func addMatches(content string, re *regexp.Regexp, seen map[string]struct{}) {
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		key := strings.TrimSpace(m[1])
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
}

func diffKeys(base, compare map[string]string) []string {
	out := make([]string, 0)
	for key := range base {
		if _, ok := compare[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func sample(items []string) []string {
	if len(items) <= 30 {
		return items
	}
	return append(items[:30], "...")
}

func placeholdersSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range reNamedPlaceholder.FindAllString(value, -1) {
		out[m] = struct{}{}
	}
	for _, m := range rePrintfPlaceholder.FindAllString(value, -1) {
		if m == "%%" {
			continue
		}
		out[m] = struct{}{}
	}
	return out
}

func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
