package memory

import "time"

type Metrics struct {
	GeneratedAt       time.Time `json:"generated_at"`
	SegmentTotal      int       `json:"segment_total"`
	SegmentOpen       int       `json:"segment_open"`
	SegmentClosed     int       `json:"segment_closed"`
	SegmentProcessing int       `json:"segment_processing"`
	SegmentPersisted  int       `json:"segment_persisted"`
	SegmentFailed     int       `json:"segment_failed"`
	FailedRate        float64   `json:"failed_rate"`
	RetryTotal        int       `json:"retry_total"`
	PendingCount      int       `json:"pending_count"`
	ReviewedCount     int       `json:"reviewed_count"`
	LastPersistedAt   time.Time `json:"last_persisted_at,omitempty"`
	WarningFailRate   bool      `json:"warning_fail_rate"`
	WarningPending    bool      `json:"warning_pending"`
	WarningRetry      bool      `json:"warning_retry"`
}

func (s *Store) GetMetrics() Metrics {
	segments := s.ListSegments(0)
	nodes := s.ListNodes(0)

	metrics := Metrics{GeneratedAt: time.Now().UTC()}
	for _, seg := range segments {
		metrics.SegmentTotal++
		metrics.RetryTotal += seg.RetryCount
		switch seg.Status {
		case SegmentStatusOpen:
			metrics.SegmentOpen++
		case SegmentStatusClosed:
			metrics.SegmentClosed++
		case SegmentStatusProcessing:
			metrics.SegmentProcessing++
		case SegmentStatusPersisted:
			metrics.SegmentPersisted++
			if seg.UpdatedAt.After(metrics.LastPersistedAt) {
				metrics.LastPersistedAt = seg.UpdatedAt
			}
		case SegmentStatusFailed:
			metrics.SegmentFailed++
		}
	}
	if metrics.SegmentTotal > 0 {
		metrics.FailedRate = float64(metrics.SegmentFailed) / float64(metrics.SegmentTotal)
	}
	for _, node := range nodes {
		if node.Type != NodeTypeFile {
			continue
		}
		if hasPrefix(node.Path, "/inbox/pending/") {
			metrics.PendingCount++
		}
		if hasPrefix(node.Path, "/inbox/reviewed/") {
			metrics.ReviewedCount++
		}
	}

	metrics.WarningFailRate = metrics.FailedRate >= 0.20 && metrics.SegmentTotal >= 5
	metrics.WarningPending = metrics.PendingCount >= 15
	metrics.WarningRetry = metrics.RetryTotal >= 10
	return metrics
}

func hasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix
}
