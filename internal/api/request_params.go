package api

import (
	"net/http"
	"strconv"
	"time"
)

func parseLimit(raw string, fallback, max int) int {
	limit := fallback
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if max > 0 && limit > max {
		return max
	}
	return limit
}

func parseOffset(raw string) int {
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func clusterIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.URL.Query().Get("clusterId")
}

func parseTimeRange(r *http.Request, fallback time.Duration) (time.Time, time.Time, error) {
	if r == nil {
		return time.Time{}, time.Time{}, nil
	}
	q := r.URL.Query()

	if lookback := q.Get("lookback"); lookback != "" {
		d, err := time.ParseDuration(lookback)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end := time.Now().UTC()
		return end.Add(-d), end, nil
	}

	start, err := parseTimestamp(q.Get("start"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseTimestamp(q.Get("end"))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if start.IsZero() || end.IsZero() {
		if fallback <= 0 {
			return time.Time{}, time.Time{}, nil
		}
		now := time.Now().UTC()
		return now.Add(-fallback), now, nil
	}
	return start, end, nil
}

func parseTimestamp(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(parsed, 0).UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}
