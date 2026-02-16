package docs

import (
	"errors"
	"strings"
)

type ClassificationLevel int

const (
	ClassificationPublic ClassificationLevel = iota
	ClassificationInternal
	ClassificationConfidential
	ClassificationRestricted
	ClassificationSecret
	ClassificationTopSecret
	ClassificationSpecialImportance
)

var levelNames = map[string]ClassificationLevel{
	"PUBLIC":             ClassificationPublic,
	"INTERNAL":           ClassificationInternal,
	"CONFIDENTIAL":       ClassificationConfidential,
	"RESTRICTED":         ClassificationRestricted,
	"SECRET":             ClassificationSecret,
	"TOP_SECRET":         ClassificationTopSecret,
	"SPECIAL_IMPORTANCE": ClassificationSpecialImportance,
}

var levelCodes = map[ClassificationLevel]string{
	ClassificationPublic:             "PUB",
	ClassificationInternal:           "INT",
	ClassificationConfidential:       "CONF",
	ClassificationRestricted:         "DSP",
	ClassificationSecret:             "SEC",
	ClassificationTopSecret:          "TOP",
	ClassificationSpecialImportance:  "SI",
}

const (
	StatusDraft    = "draft"
	StatusReview   = "review"
	StatusApproved = "approved"
	StatusReturned = "returned"
)

const (
	FormatMarkdown = "md"
	FormatDocx     = "docx"
	FormatPDF      = "pdf"
	FormatTXT      = "txt"
	FormatJSON     = "json"
)

var TagList = []string{"COMMERCIAL_SECRET", "PERSONAL_DATA"}

func ParseLevel(s string) (ClassificationLevel, error) {
	up := strings.ToUpper(strings.TrimSpace(s))
	l, ok := levelNames[up]
	if !ok {
		return ClassificationPublic, errors.New("unknown classification level")
	}
	return l, nil
}

func LevelCode(level ClassificationLevel) string {
	if c, ok := levelCodes[level]; ok {
		return c
	}
	return "UNK"
}

func LevelName(level ClassificationLevel) string {
	for k, v := range levelNames {
		if v == level {
			return k
		}
	}
	return "UNKNOWN"
}

func RequiresWatermark(level ClassificationLevel, threshold ClassificationLevel) bool {
	return level >= threshold
}

func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, t := range tags {
		tt := strings.ToUpper(strings.TrimSpace(t))
		if tt == "" {
			continue
		}
		if _, ok := seen[tt]; ok {
			continue
		}
		out = append(out, tt)
		seen[tt] = struct{}{}
	}
	return out
}

func HasClearance(userLevel ClassificationLevel, userTags []string, docLevel ClassificationLevel, docTags []string) bool {
	if userLevel < docLevel {
		return false
	}
	normalizedTags := map[string]struct{}{}
	for _, tag := range userTags {
		val := strings.ToUpper(strings.TrimSpace(tag))
		if val == "" {
			continue
		}
		normalizedTags[val] = struct{}{}
	}
	for _, t := range docTags {
		if _, ok := normalizedTags[strings.ToUpper(strings.TrimSpace(t))]; !ok {
			return false
		}
	}
	return true
}
