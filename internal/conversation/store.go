package conversation

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
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
	ID      string `json:"id"`
	Title   string `json:"title"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
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

	metaConversationState = "conversation_state"
)

type archiveRecord struct {
	ID                string               `json:"id"`
	CreatedAt         time.Time            `json:"created_at"`
	TrimmedMessageCnt int                  `json:"trimmed_message_count"`
	KeySummary        []string             `json:"key_summary"`
	Sections          []archiveSectionItem `json:"sections"`
}

type archiveSectionItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
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

func (s *Store) Append(role, content string) {
	s.AppendWithUsage(role, content, nil)
}

func (s *Store) AppendAssistant(content string, usage *TokenUsage) {
	s.AppendWithUsage("assistant", content, usage)
}

func (s *Store) AppendWithUsage(role, content string, usage *TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, Message{
		Role:      role,
		Content:   content,
		Usage:     normalizeTokenUsage(usage),
		CreatedAt: time.Now(),
	})
	_ = s.persistLocked()
}

func (s *Store) SetLatestUserToolCalls(toolCalls []ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.messages) == 0 || s.messages[len(s.messages)-1].Role != "user" {
		return fmt.Errorf("no pending user message")
	}
	s.messages[len(s.messages)-1].ToolCalls = normalizeToolCalls(toolCalls)
	return s.persistLocked()
}

func (s *Store) Snapshot() (string, []Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary, cloneMessages(s.messages)
}

func (s *Store) SnapshotWithEvents() (string, []Message, []Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary, cloneMessages(s.messages), cloneEvents(s.events)
}

func (s *Store) AppendEvent(eventType, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventType = strings.TrimSpace(eventType)
	content = strings.TrimSpace(content)
	if eventType == "" || content == "" {
		return
	}
	s.events = append(s.events, Event{
		Type:      eventType,
		Content:   content,
		CreatedAt: time.Now(),
	})
	if len(s.events) > maxArchiveEvents {
		s.events = append([]Event(nil), s.events[len(s.events)-maxArchiveEvents:]...)
	}
	_ = s.persistLocked()
}

func (s *Store) SetSummaryAndTrim(summary string, keepRecent int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary = strings.TrimSpace(summary)
	if keepRecent < 0 {
		keepRecent = 0
	}
	if len(s.messages) <= keepRecent {
		s.summary = summary
		_ = s.persistLocked()
		return
	}

	trimmed := cloneMessages(s.messages[:len(s.messages)-keepRecent])
	s.messages = append([]Message(nil), s.messages[len(s.messages)-keepRecent:]...)

	if ref, err := s.createArchiveLocked(trimmed); err == nil {
		summary = appendArchiveRefToSummary(summary, ref)
	}
	s.summary = summary
	_ = s.persistLocked()
}

func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.summary = ""
	s.messages = nil
	s.events = nil
	return s.persistLocked()
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

func (s *Store) ReadArchiveIndex(archiveID string) (ArchiveIndex, error) {
	archiveID = strings.TrimSpace(archiveID)
	if archiveID == "" {
		return ArchiveIndex{}, ErrArchiveNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, err := s.readArchiveRecordLocked(archiveID)
	if err != nil {
		return ArchiveIndex{}, err
	}
	sections := make([]ArchiveSectionMeta, 0, len(record.Sections))
	for _, sec := range record.Sections {
		sections = append(sections, ArchiveSectionMeta{
			ID:     sec.ID,
			Title:  sec.Title,
			Digest: sec.Digest,
		})
	}
	title := ""
	digest := ""
	if len(record.Sections) > 0 {
		title = record.Sections[0].Title
		digest = record.Sections[0].Digest
	}
	return ArchiveIndex{
		ArchiveID:  record.ID,
		DBRef:      record.ID,
		Title:      title,
		Digest:     digest,
		KeySummary: append([]string(nil), record.KeySummary...),
		Sections:   sections,
		CreatedAt:  record.CreatedAt,
	}, nil
}

func (s *Store) ReadArchiveSection(archiveID, sectionID string) (ArchiveSection, error) {
	archiveID = strings.TrimSpace(archiveID)
	sectionID = strings.TrimSpace(sectionID)
	if archiveID == "" || sectionID == "" {
		return ArchiveSection{}, ErrArchiveSectionNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, err := s.readArchiveRecordLocked(archiveID)
	if err != nil {
		return ArchiveSection{}, err
	}
	for _, sec := range record.Sections {
		if strings.EqualFold(strings.TrimSpace(sec.ID), sectionID) {
			return ArchiveSection{
				ID:      sec.ID,
				Title:   sec.Title,
				Digest:  sec.Digest,
				Content: sec.Content,
			}, nil
		}
	}
	return ArchiveSection{}, ErrArchiveSectionNotFound
}

func (s *Store) bootstrap() error {
	if s.db == nil {
		return nil
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketMeta)); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketArchives)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("init conversation db schema: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}

		state := meta.Get([]byte(metaConversationState))
		if state == nil {
			seed := payload{}
			encoded, err := json.Marshal(seed)
			if err != nil {
				return err
			}
			if err := meta.Put([]byte(metaConversationState), encoded); err != nil {
				return err
			}
			s.summary = seed.Summary
			s.messages = seed.Messages
			s.events = seed.Events
			return nil
		}

		var loaded payload
		if err := json.Unmarshal(state, &loaded); err != nil {
			return err
		}
		s.summary = strings.TrimSpace(loaded.Summary)
		s.messages = cloneMessages(loaded.Messages)
		s.events = normalizeEvents(loaded.Events)
		return nil
	})
}

func (s *Store) persistLocked() error {
	if s.db == nil {
		return nil
	}
	state := payload{
		Summary:  s.summary,
		Messages: cloneMessages(s.messages),
		Events:   cloneEvents(s.events),
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode conversation state: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return fmt.Errorf("conversation db schema missing")
		}
		return meta.Put([]byte(metaConversationState), encoded)
	})
}

func (s *Store) readArchiveRecordLocked(archiveID string) (archiveRecord, error) {
	if s.db == nil {
		return archiveRecord{}, ErrArchiveNotFound
	}
	var rec archiveRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketArchives))
		if b == nil {
			return ErrArchiveNotFound
		}
		raw := b.Get([]byte(archiveID))
		if raw == nil {
			return ErrArchiveNotFound
		}
		return json.Unmarshal(raw, &rec)
	})
	if err != nil {
		return archiveRecord{}, err
	}
	return rec, nil
}

func (s *Store) createArchiveLocked(trimmed []Message) (archiveRef, error) {
	if s.db == nil || len(trimmed) == 0 {
		return archiveRef{}, fmt.Errorf("archive unavailable")
	}
	sections := buildArchiveSections(trimmed)
	if len(sections) == 0 {
		return archiveRef{}, fmt.Errorf("no archive sections")
	}
	id := buildArchiveID(trimmed, time.Now())

	keySummary := make([]string, 0, minInt(6, len(sections)))
	for i := 0; i < len(sections) && i < 6; i++ {
		keySummary = append(keySummary, fmt.Sprintf("[%s] %s", sections[i].ID, sections[i].Digest))
	}
	record := archiveRecord{
		ID:                id,
		CreatedAt:         time.Now().UTC(),
		TrimmedMessageCnt: len(trimmed),
		KeySummary:        keySummary,
		Sections:          sections,
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return archiveRef{}, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketArchives))
		if b == nil {
			return fmt.Errorf("archives bucket missing")
		}
		return b.Put([]byte(id), encoded)
	}); err != nil {
		return archiveRef{}, err
	}

	sectionHeads := make([]string, 0, len(sections))
	for _, sec := range sections {
		sectionHeads = append(sectionHeads, fmt.Sprintf("[%s] %s", sec.ID, sanitizeField(sec.Title)))
	}
	digest := ""
	if len(keySummary) > 0 {
		digest = strings.Join(keySummary[:minInt(3, len(keySummary))], " ; ")
	}
	title := sanitizeField(sections[0].Title)
	return archiveRef{
		ID:       id,
		DBRef:    id,
		Title:    title,
		Sections: strings.Join(sectionHeads, " ; "),
		Digest:   sanitizeField(digest),
	}, nil
}

func buildArchiveSections(trimmed []Message) []archiveSectionItem {
	if len(trimmed) == 0 {
		return nil
	}
	const chunkSize = 4
	out := make([]archiveSectionItem, 0, (len(trimmed)+chunkSize-1)/chunkSize)
	for start := 0; start < len(trimmed); start += chunkSize {
		end := start + chunkSize
		if end > len(trimmed) {
			end = len(trimmed)
		}
		chunk := trimmed[start:end]
		sectionID := "S" + strconv.Itoa(len(out)+1)
		title := deriveArchiveSectionTitle(chunk, len(out)+1)
		digest := deriveArchiveSectionDigest(chunk)
		var content strings.Builder
		for i, msg := range chunk {
			content.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, strings.TrimSpace(msg.Role), normalizeArchiveText(msg.Content, 240)))
		}
		out = append(out, archiveSectionItem{
			ID:      sectionID,
			Title:   title,
			Digest:  digest,
			Content: strings.TrimSpace(content.String()),
		})
	}
	return out
}

func buildArchiveID(trimmed []Message, now time.Time) string {
	var seed strings.Builder
	seed.WriteString(now.UTC().Format(time.RFC3339Nano))
	for _, msg := range trimmed {
		seed.WriteString("|")
		seed.WriteString(strings.TrimSpace(msg.Role))
		seed.WriteString(":")
		seed.WriteString(strings.TrimSpace(msg.Content))
	}
	sum := sha1.Sum([]byte(seed.String()))
	return fmt.Sprintf("arc-%s-%s", now.UTC().Format("20060102-150405"), hex.EncodeToString(sum[:])[:8])
}

func deriveArchiveSectionTitle(chunk []Message, index int) string {
	for _, msg := range chunk {
		if strings.TrimSpace(strings.ToLower(msg.Role)) == "user" {
			if v := normalizeArchiveText(msg.Content, 24); v != "" {
				return v
			}
		}
	}
	for _, msg := range chunk {
		if v := normalizeArchiveText(msg.Content, 24); v != "" {
			return v
		}
	}
	return fmt.Sprintf("对话片段 %d", index)
}

func deriveArchiveSectionDigest(chunk []Message) string {
	for _, msg := range chunk {
		if v := normalizeArchiveText(msg.Content, 42); v != "" {
			return v
		}
	}
	return "(无摘要)"
}

func appendArchiveRefToSummary(summary string, ref archiveRef) string {
	summary = strings.TrimSpace(summary)
	if strings.TrimSpace(ref.ID) == "" {
		return summary
	}

	base, refs := splitSummaryArchiveRefs(summary)
	merged := make([]archiveRef, 0, len(refs)+1)
	merged = append(merged, ref)
	seen := map[string]struct{}{ref.ID: {}}
	for _, old := range refs {
		id := strings.TrimSpace(old.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, old)
		if len(merged) >= maxArchiveRefsInView {
			break
		}
	}
	block := renderArchiveIndexBlock(merged)
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimSpace(base + "\n\n" + block)
}

func splitSummaryArchiveRefs(summary string) (string, []archiveRef) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", nil
	}
	begin := strings.Index(summary, archiveIndexBeginTag)
	end := strings.Index(summary, archiveIndexEndTag)
	if begin < 0 || end < 0 || end < begin {
		return summary, nil
	}
	block := summary[begin : end+len(archiveIndexEndTag)]
	lines := strings.Split(block, "\n")
	refs := make([]archiveRef, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- archive_id=") {
			continue
		}
		if ref, ok := parseArchiveRefLine(line); ok {
			refs = append(refs, ref)
		}
	}
	base := strings.TrimSpace(summary[:begin] + summary[end+len(archiveIndexEndTag):])
	return base, refs
}

func parseArchiveRefLine(line string) (archiveRef, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
	parts := strings.Split(line, "|")
	ref := archiveRef{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		switch key {
		case "archive_id":
			ref.ID = value
		case "db_ref":
			ref.DBRef = value
		case "title":
			ref.Title = value
		case "sections":
			ref.Sections = value
		case "digest":
			ref.Digest = value
		}
	}
	return ref, strings.TrimSpace(ref.ID) != ""
}

func renderArchiveIndexBlock(refs []archiveRef) string {
	if len(refs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(archiveIndexBeginTag + "\n")
	b.WriteString("历史归档索引（仅标题级，先按 archive_id 拉索引，再按 section_id 拉内容）：\n")
	for i, ref := range refs {
		if i >= maxArchiveRefsInView {
			break
		}
		b.WriteString("- archive_id=" + sanitizeField(ref.ID))
		b.WriteString(" | db_ref=" + sanitizeField(ref.DBRef))
		b.WriteString(" | title=" + sanitizeField(ref.Title))
		b.WriteString(" | sections=" + sanitizeField(trimRunesString(ref.Sections, 180)))
		b.WriteString(" | digest=" + sanitizeField(trimRunesString(ref.Digest, 160)))
		b.WriteString("\n")
	}
	b.WriteString(archiveIndexEndTag)
	return strings.TrimSpace(b.String())
}

func cloneMessages(in []Message) []Message {
	if len(in) == 0 {
		return nil
	}
	out := make([]Message, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ToolCalls = cloneToolCalls(in[i].ToolCalls)
		out[i].Usage = normalizeTokenUsage(in[i].Usage)
	}
	return out
}

func cloneToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, len(in))
	copy(out, in)
	return out
}

func cloneEvents(in []Event) []Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]Event, len(in))
	copy(out, in)
	return out
}

func normalizeToolCalls(in []ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(in))
	for _, call := range in {
		call.ID = strings.TrimSpace(call.ID)
		call.Name = strings.TrimSpace(call.Name)
		call.Arguments = strings.TrimSpace(call.Arguments)
		call.Result = strings.TrimSpace(call.Result)
		call.Error = strings.TrimSpace(call.Error)
		if call.Name == "" {
			call.Name = "(unknown)"
		}
		if call.Arguments == "" {
			call.Arguments = "{}"
		}
		if call.CreatedAt.IsZero() {
			call.CreatedAt = time.Now()
		}
		out = append(out, call)
	}
	return out
}

func normalizeEvents(in []Event) []Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]Event, 0, len(in))
	for _, evt := range in {
		evt.Type = strings.TrimSpace(evt.Type)
		evt.Content = strings.TrimSpace(evt.Content)
		if evt.Type == "" || evt.Content == "" {
			continue
		}
		if evt.CreatedAt.IsZero() {
			evt.CreatedAt = time.Now()
		}
		out = append(out, evt)
	}
	if len(out) > maxArchiveEvents {
		out = append([]Event(nil), out[len(out)-maxArchiveEvents:]...)
	}
	return out
}

func normalizeTokenUsage(in *TokenUsage) *TokenUsage {
	if in == nil {
		return nil
	}
	usage := TokenUsage{
		PromptTokens:     max(0, in.PromptTokens),
		CompletionTokens: max(0, in.CompletionTokens),
		TotalTokens:      max(0, in.TotalTokens),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return &usage
}

func normalizeArchiveText(text string, maxRunes int) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	return trimRunesString(text, maxRunes)
}

func sanitizeField(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "|", "/")
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.Join(strings.Fields(v), " ")
	if v == "" {
		return "(none)"
	}
	return v
}

func trimRunesString(input string, max int) string {
	input = strings.TrimSpace(input)
	if max <= 0 || input == "" {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return strings.TrimSpace(string(runes[:max-3])) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
