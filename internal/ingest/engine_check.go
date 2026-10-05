package ingest

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type EngineObservation struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type CategoryCheck struct {
	Instance  int                 `json:"instance"`
	Category  string              `json:"category"`
	Status    string              `json:"status"`
	Code      string              `json:"code,omitempty"`
	Results   int                 `json:"results"`
	Engines   []EngineObservation `json:"engines"`
	Truncated bool                `json:"truncated"`
}

type EngineCheckReport struct {
	Categories          []CategoryCheck `json:"categories"`
	ConfiguredInstances int             `json:"configured_instances"`
	CheckedInstances    int             `json:"checked_instances"`
	Error               string          `json:"error,omitempty"`
}

// The scheduler serializes this diagnostic with ingestion. These sample searches
// never ingest results and cannot establish health for engines that returned none.
func (s *Service) CheckEngines(ctx context.Context) EngineCheckReport {
	s.searchStarted, s.searchPaused, s.searchMinDelay = false, 0, 5*time.Second
	defer func() { s.searchMinDelay = 0 }()
	report := EngineCheckReport{Categories: []CategoryCheck{}, ConfiguredInstances: len(s.cfg.SearxngInstances)}
	if len(s.cfg.SearxngInstances) == 0 {
		report.Error = "no_instances"
		return report
	}
	for i, base := range s.cfg.SearxngInstances {
		if i == 3 {
			break
		}
		report.CheckedInstances++
		for _, category := range []string{"news", "general"} {
			if ctx.Err() != nil {
				report.Error = errorCode(ctx.Err())
				return report
			}
			row := CategoryCheck{Instance: i + 1, Category: category, Status: "failed", Engines: []EngineObservation{}}
			if _, blocked := s.blockRemaining(base); blocked {
				row.Code = "instance_cooldown"
			} else {
				parsed, retry, err := s.fetchSearchResponse(ctx, base, "intel cpu", category, "day", 1)
				if retry > 0 {
					s.setBlocked(base, retry)
				}
				row.Results = len(parsed.Results)
				row.Engines, row.Truncated = observations(parsed)
				if err != nil {
					if f, ok := err.(Failure); ok {
						row.Code = f.Code
					} else {
						row.Code = errorCode(err)
					}
					if strings.HasPrefix(row.Code, "engine_errors_") && parsed.Results != nil {
						row.Status = "warnings"
					}
				} else if len(parsed.Results) == 0 {
					row.Status = "empty"
				} else {
					row.Status = "results"
				}
			}
			report.Categories = append(report.Categories, row)
		}
	}
	if ctx.Err() != nil {
		report.Error = errorCode(ctx.Err())
	}
	return report
}

func observations(parsed searxResponse) ([]EngineObservation, bool) {
	const cap = 50
	states := map[string]string{}
	truncated := false
	add := func(raw, status string) {
		name := engineName(raw)
		if _, exists := states[name]; !exists && len(states) >= cap {
			truncated = true
			return
		}
		states[name] = status
	}
	for _, entry := range parsed.Results {
		for _, engine := range entry.Engines {
			add(engine, "returned_results")
		}
	}
	for _, raw := range parsed.UnresponsiveEngines {
		var pair []string
		if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 {
			add("unknown", "engine_error")
			continue
		}
		add(pair[0], engineWarning(pair[1]))
	}
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]EngineObservation, 0, len(names))
	for _, name := range names {
		out = append(out, EngineObservation{Name: name, Status: states[name]})
	}
	return out, truncated
}

func engineName(raw string) string {
	if len(raw) == 0 || len(raw) > 64 {
		return "unknown"
	}
	for _, r := range raw {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(" ._-", r)) {
			return "unknown"
		}
	}
	return raw
}

func engineWarning(raw string) string {
	msg := strings.ToLower(raw)
	switch {
	case strings.Contains(msg, "captcha"):
		return "captcha"
	case strings.Contains(msg, "too many requests") || strings.Contains(msg, "429"):
		return "rate_limited"
	case strings.Contains(msg, "access denied") || strings.Contains(msg, "forbidden"):
		return "access_denied"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "http error"):
		return "http_error"
	case strings.Contains(msg, "suspend"):
		return "suspended"
	default:
		return "engine_error"
	}
}
