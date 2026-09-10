package ingest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

const maxFailureDetails = 32

// Failure contains only bounded diagnostic identifiers, never remote URLs,
// query text, response bodies or database error messages.
type Failure struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	TopicID   int64  `json:"topic_id,omitempty"`
	ArticleID int64  `json:"article_id,omitempty"`
	Instance  int    `json:"instance,omitempty"`
	Category  string `json:"category,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Page      int    `json:"page,omitempty"`
	cause     error
}

func (f Failure) Error() string { return f.Stage + ": " + f.Code }
func (f Failure) Unwrap() error { return f.cause }

// PartialRunError reports failures even when useful results were persisted.
// Total and Counts include omitted details; Failures is capped to prevent spam.
type PartialRunError struct {
	Total    int            `json:"total"`
	Counts   map[string]int `json:"counts"`
	Failures []Failure      `json:"failures"`
	canceled error
}

func (e *PartialRunError) Error() string {
	keys := make([]string, 0, len(e.Counts))
	for key := range e.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, e.Counts[key]))
	}
	return fmt.Sprintf("ingest incomplete: %d failure(s) (%s)", e.Total, strings.Join(parts, ", "))
}
func (e *PartialRunError) Unwrap() []error {
	out := make([]error, 0, len(e.Failures)+1)
	for _, f := range e.Failures {
		out = append(out, f)
	}
	if e.canceled != nil {
		out = append(out, e.canceled)
	}
	return out
}
func (e *PartialRunError) add(f Failure) {
	e.Total++
	if e.Counts == nil {
		e.Counts = make(map[string]int)
	}
	e.Counts[f.Stage]++
	if len(e.Failures) < maxFailureDetails {
		e.Failures = append(e.Failures, f)
	}
	if errors.Is(f.cause, context.Canceled) {
		e.canceled = context.Canceled
	}
	if errors.Is(f.cause, context.DeadlineExceeded) {
		e.canceled = context.DeadlineExceeded
	}
}
func (e *PartialRunError) merge(err error, topicID int64, instance int) {
	if err == nil {
		return
	}
	var partial *PartialRunError
	if errors.As(err, &partial) {
		for _, f := range partial.Failures {
			if topicID != 0 {
				f.TopicID = topicID
			}
			if instance != 0 {
				f.Instance = instance
			}
			e.add(f)
		}
		e.Total += partial.Total - len(partial.Failures)
		for stage, count := range partial.Counts {
			for _, f := range partial.Failures {
				if f.Stage == stage {
					count--
				}
			}
			e.Counts[stage] += count
		}
		if partial.canceled != nil {
			e.canceled = partial.canceled
		}
		return
	}
	var failure Failure
	if errors.As(err, &failure) {
		failure.TopicID = topicID
		if instance != 0 {
			failure.Instance = instance
		}
		e.add(failure)
		return
	}
	e.add(Failure{Stage: "fetch", Code: errorCode(err), TopicID: topicID, Instance: instance, cause: err})
}
func (e *PartialRunError) err() error {
	if e.Total == 0 {
		return nil
	}
	return e
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, errNonPublicAddress):
		return "non_public_destination"
	case errors.Is(err, errInvalidArticleURL):
		return "invalid_url"
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return "timeout"
	}
	return "operation_failed"
}
func dbFailure(stage string, err error) Failure {
	return Failure{Stage: stage, Code: errorCode(err), cause: err}
}
