package memory

import (
	"context"
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"laughing-barnacle/internal/conversation"
	"regexp"
	"strings"
	"sync"
	"time"
)

type NodeType string

const (
	NodeTypeDir  NodeType = "dir"
	NodeTypeFile NodeType = "file"
)

type SegmentStatus string

const (
	SegmentStatusOpen       SegmentStatus = "open"
	SegmentStatusClosed     SegmentStatus = "closed"
	SegmentStatusProcessing SegmentStatus = "processing"
	SegmentStatusPersisted  SegmentStatus = "persisted"
	SegmentStatusFailed     SegmentStatus = "failed"
)

const (
	bucketNodes    = "memory_nodes"
	bucketChildren = "memory_children"
	bucketSegments = "memory_segments"
	bucketMeta     = "memory_meta"
	bucketAudit    = "memory_audit"

	metaOpenSegmentID = "open_segment_id"
)

var (
	ErrNodeNotFound     = errors.New("memory node not found")
	ErrSectionNotFound  = errors.New("memory section not found")
	ErrPathConflict     = errors.New("memory path conflict")
	ErrRevisionConflict = errors.New("memory revision conflict")

	pathSegPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type Ref struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Section struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
}

type FileContent struct {
	Summary  string    `json:"summary"`
	Facts    []string  `json:"facts,omitempty"`
	Sections []Section `json:"sections,omitempty"`
	Refs     []Ref     `json:"refs,omitempty"`
}

type Node struct {
	ID            string       `json:"id"`
	Path          string       `json:"path"`
	Title         string       `json:"title"`
	Type          NodeType     `json:"type"`
	SchemaKind    string       `json:"schema_kind"`
	SchemaVersion int          `json:"schema_version"`
	Tags          []string     `json:"tags,omitempty"`
	Source        string       `json:"source,omitempty"`
	Confidence    float64      `json:"confidence,omitempty"`
	Revision      int64        `json:"revision"`
	Content       *FileContent `json:"content,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

type IndexItem struct {
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Type      NodeType  `json:"type"`
	Summary   string    `json:"summary,omitempty"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type UpsertRequest struct {
	Mode             string
	Path             string
	Title            string
	Type             NodeType
	SchemaKind       string
	SchemaVersion    int
	Tags             []string
	Source           string
	Confidence       float64
	Summary          string
	Facts            []string
	Sections         []Section
	Refs             []Ref
	ExpectedRevision int64
}

type SegmentTurn struct {
	User      string                  `json:"user"`
	Assistant string                  `json:"assistant"`
	ToolCalls []conversation.ToolCall `json:"tool_calls,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

type Segment struct {
	ID             string        `json:"id"`
	Status         SegmentStatus `json:"status"`
	RetryCount     int           `json:"retry_count,omitempty"`
	Turns          []SegmentTurn `json:"turns"`
	StartedAt      time.Time     `json:"started_at"`
	LastUserAt     time.Time     `json:"last_user_at"`
	LastActivityAt time.Time     `json:"last_activity_at"`
	ClosedAt       time.Time     `json:"closed_at,omitempty"`
	CloseReason    string        `json:"close_reason,omitempty"`
	PersistedPaths []string      `json:"persisted_paths,omitempty"`
	Error          string        `json:"error,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type Store struct {
	mu                 sync.RWMutex
	path               string
	db                 *bolt.DB
	extractor          SegmentExtractor
	extractionFallback bool
}

func NewStoreWithFile(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("memory store path is required")
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open memory db: %w", err)
	}
	s := &Store{path: path, db: db, extractionFallback: true}
	if err := s.bootstrap(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) SetSegmentExtractor(extractor SegmentExtractor, fallback bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extractor = extractor
	s.extractionFallback = fallback
}

func (s *Store) extractSegmentLocked(seg Segment) (SegmentExtraction, error) {
	rule := buildRuleBasedSegmentExtraction(seg)
	if s.extractor == nil {
		return rule, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	extracted, err := s.extractor.ExtractSegment(ctx, seg)
	if err != nil {
		if s.extractionFallback {
			return rule, nil
		}
		return SegmentExtraction{}, err
	}
	if strings.TrimSpace(extracted.ProjectSummary) == "" {
		extracted.ProjectSummary = rule.ProjectSummary
	}
	if len(extracted.ProjectFacts) == 0 {
		extracted.ProjectFacts = rule.ProjectFacts
	}
	if len(extracted.Candidates) == 0 {
		extracted.Candidates = rule.Candidates
	}
	return extracted, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Store) bootstrap() error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketNodes, bucketChildren, bucketSegments, bucketMeta, bucketAudit} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("init memory db schema: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seedDefaultNamespacesLocked()
}
