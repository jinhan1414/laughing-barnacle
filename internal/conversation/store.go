package conversation

import (
	"errors"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"strings"
	"sync"
	"time"
)

type ToolCall struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name"`
	Arguments string    `json:"arguments,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
}

// Message is one conversation record kept in memory.
type Message struct {
	Role      string      `json:"role"`
	Content   string      `json:"content"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
	Usage     *TokenUsage `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

type payload struct {
	Summary  string    `json:"summary"`
	Messages []Message `json:"messages"`
	Events   []Event   `json:"events,omitempty"`
}

type ArchiveSectionMeta struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Digest string `json:"digest"`
}

type ArchiveIndex struct {
	ArchiveID  string               `json:"archive_id"`
	DBRef      string               `json:"db_ref"`
	Title      string               `json:"title"`
	Digest     string               `json:"digest"`
	KeySummary []string             `json:"key_summary,omitempty"`
	Sections   []ArchiveSectionMeta `json:"sections"`
	CreatedAt  time.Time            `json:"created_at"`
}

type ArchiveSection struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	Digest           string                 `json:"digest"`
	Content          string                 `json:"content"`
	Messages         []ArchiveReplayMessage `json:"messages,omitempty"`
	LegacyIncomplete bool                   `json:"legacy_incomplete,omitempty"`
}

type ArchiveReplayMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

var (
	ErrArchiveNotFound        = errors.New("archive not found")
	ErrArchiveSectionNotFound = errors.New("archive section not found")
)

const (
	maxArchiveRefsInView = 6
	maxArchiveEvents     = 200
	archiveIndexBeginTag = "[archive_index_begin]"
	archiveIndexEndTag   = "[archive_index_end]"

	bucketMeta     = "meta"
	bucketArchives = "archives"

	metaConversationState  = "conversation_state"
	metaAsyncTaskState     = "async_task_state"
	metaChatTurnState      = "chat_turn_state"
	metaAutonomousRunState = "autonomous_run_state"
)

type archiveRecord struct {
	ID                string               `json:"id"`
	CreatedAt         time.Time            `json:"created_at"`
	TrimmedMessageCnt int                  `json:"trimmed_message_count"`
	KeySummary        []string             `json:"key_summary"`
	Sections          []archiveSectionItem `json:"sections"`
}

type archiveSectionItem struct {
	ID       string                 `json:"id"`
	Title    string                 `json:"title"`
	Digest   string                 `json:"digest"`
	Content  string                 `json:"content"`
	Messages []ArchiveReplayMessage `json:"messages,omitempty"`
}

type archiveRef struct {
	ID       string
	DBRef    string
	Title    string
	Sections string
	Digest   string
}

// Store holds one global conversation (no session concept).
type Store struct {
	mu       sync.RWMutex
	path     string
	db       *bolt.DB
	summary  string
	messages []Message
	events   []Event
}

func NewStore() *Store {
	return &Store{}
}

func NewStoreWithFile(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("conversation store path is required")
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open conversation db: %w", err)
	}

	s := &Store{
		path: path,
		db:   db,
	}
	if err := s.bootstrap(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
