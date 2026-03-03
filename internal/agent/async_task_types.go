package agent

import (
	"fmt"
	"strings"
	"time"
)

const (
	asyncTaskTypeGeneric = "generic"
	asyncTaskTypeA2A     = "a2a"
)

const (
	asyncTaskStatusSubmitted = "submitted"
	asyncTaskStatusWorking   = "working"
	asyncTaskStatusSucceeded = "succeeded"
	asyncTaskStatusFailed    = "failed"
	asyncTaskStatusCanceled  = "canceled"
)

const (
	defaultAsyncTaskLogLimit = 20
	maxAsyncTaskLogLimit     = 200
	maxAsyncTaskEntries      = 300
	maxAsyncTaskLogsRetained = 500
)

type AsyncTaskSubmitInput struct {
	TaskType       string
	Request        string
	AgentID        string
	AgentInput     string
	DedupeKey      string
	NotifyOnFinish bool
	Metadata       map[string]any
}

type AsyncTaskGetInput struct {
	TaskID      string
	IncludeLogs bool
	LogCursor   int
	LogLimit    int
}

type AsyncTaskCancelInput struct {
	TaskID string
	Reason string
}

type AsyncTaskLog struct {
	Cursor    int
	CreatedAt time.Time
	Level     string
	Message   string
}

type AsyncTask struct {
	ID                         string
	TaskType                   string
	Status                     string
	TrackerState               string
	TrackerReason              string
	ProgressSummary            string
	Request                    string
	AgentID                    string
	AgentInput                 string
	RemoteTaskID               string
	DedupeKey                  string
	NotifyOnFinish             bool
	Result                     string
	Error                      string
	Metadata                   map[string]any
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	NextPollAt                 time.Time
	LastRenewedAt              time.Time
	LastReconciledAt           time.Time
	ConsecutiveErrors          int
	TrackingRenewals           int
	ReconcileSkippedByDebounce bool
	Logs                       []AsyncTaskLog
}

func normalizeAsyncTaskType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case asyncTaskTypeGeneric:
		return asyncTaskTypeGeneric, nil
	case asyncTaskTypeA2A:
		return asyncTaskTypeA2A, nil
	default:
		return "", fmt.Errorf("task_type must be one of: generic,a2a")
	}
}

func normalizeAsyncTaskLogLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultAsyncTaskLogLimit, nil
	}
	if limit < 0 {
		return 0, fmt.Errorf("log_limit must be >= 0")
	}
	if limit > maxAsyncTaskLogLimit {
		return 0, fmt.Errorf("log_limit must be <= %d", maxAsyncTaskLogLimit)
	}
	return limit, nil
}

func isAsyncTaskTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case asyncTaskStatusSucceeded, asyncTaskStatusFailed, asyncTaskStatusCanceled:
		return true
	default:
		return false
	}
}

func isA2AInProgressStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "submitted", "working", "running", "pending":
		return true
	default:
		return false
	}
}

func normalizeA2ATerminalStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "completed", "done":
		return asyncTaskStatusSucceeded
	case "failed", "error":
		return asyncTaskStatusFailed
	case "canceled", "cancelled":
		return asyncTaskStatusCanceled
	case "rejected":
		return asyncTaskStatusFailed
	default:
		return ""
	}
}

func normalizeA2ABlockedStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "input-required":
		return "a2a task requires additional input"
	case "auth-required":
		return "a2a task requires authentication"
	default:
		return ""
	}
}
