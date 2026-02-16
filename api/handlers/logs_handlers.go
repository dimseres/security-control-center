package handlers

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"berkut-scc/core/store"
)

type LogsHandler struct {
	audits store.AuditStore
}

func NewLogsHandler(audits store.AuditStore) *LogsHandler {
	return &LogsHandler{audits: audits}
}

func (h *LogsHandler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.audits == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []store.AuditRecord{}})
		return
	}
	filter := parseLogFilter(r)
	items, err := h.filteredLogs(r, filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"filter": filter,
	})
}

func (h *LogsHandler) Export(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.audits == nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	filter := parseLogFilter(r)
	if filter.Limit <= 0 || filter.Limit > 5000 {
		filter.Limit = 5000
	}
	items, err := h.filteredLogs(r, filter)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	filename := "event_feed_" + time.Now().UTC().Format("20060102_150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"time", "username", "section", "action", "details"})
	for i := range items {
		_ = writer.Write([]string{
			items[i].CreatedAt.UTC().Format(time.RFC3339),
			strings.TrimSpace(items[i].Username),
			logCategory(items[i].Action),
			strings.TrimSpace(items[i].Action),
			strings.TrimSpace(items[i].Details),
		})
	}
	writer.Flush()
}

type logFilter struct {
	Section string
	Action  string
	User    string
	Query   string
	Since   time.Time
	To      *time.Time
	Limit   int
}

func parseLogFilter(r *http.Request) logFilter {
	q := r.URL.Query()
	since := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if rawSince := strings.TrimSpace(q.Get("since")); rawSince != "" {
		if parsed, err := parseDateTime(rawSince); err == nil && !parsed.IsZero() {
			since = parsed.UTC()
		}
	}
	var until *time.Time
	if rawTo := strings.TrimSpace(q.Get("to")); rawTo != "" {
		if parsed, err := parseDateTime(rawTo); err == nil && !parsed.IsZero() {
			t := parsed.UTC()
			until = &t
		}
	}
	limit := 1000
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 5000 {
		limit = 5000
	}
	return logFilter{
		Section: strings.ToLower(strings.TrimSpace(q.Get("section"))),
		Action:  strings.ToLower(strings.TrimSpace(q.Get("action"))),
		User:    strings.ToLower(strings.TrimSpace(q.Get("user"))),
		Query:   strings.ToLower(strings.TrimSpace(q.Get("q"))),
		Since:   since,
		To:      until,
		Limit:   limit,
	}
}

func parseDateTime(raw string) (time.Time, error) {
	val := strings.TrimSpace(raw)
	if val == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, val); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, strconv.ErrSyntax
}
