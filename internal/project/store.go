package project

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var projectIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const (
	bucketProjects      = "projects"
	maxListItemsPerType = 32
	maxItemRunes        = 180
)

type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Goal       string    `json:"goal,omitempty"`
	Status     string    `json:"status,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	KeyFacts   []string  `json:"key_facts,omitempty"`
	Milestones []string  `json:"milestones,omitempty"`
	Risks      []string  `json:"risks,omitempty"`
	Todos      []string  `json:"todos,omitempty"`
	Decisions  []string  `json:"decisions,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	db   *bolt.DB
}

func NewStoreWithFile(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("project store path is required")
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open project db: %w", err)
	}
	s := &Store{path: path, db: db}
	if err := s.bootstrap(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
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

func (s *Store) ListProjects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items, err := s.listProjectsLocked()
	if err != nil {
		return nil
	}
	return items
}

func (s *Store) ReadProject(id string) (Project, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Project{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok, err := s.readProjectLocked(id)
	if err != nil || !ok {
		return Project{}, false
	}
	return item, true
}

func (s *Store) UpsertProject(input Project) (Project, error) {
	input = normalizeProject(input)
	if strings.TrimSpace(input.Name) == "" {
		return Project{}, fmt.Errorf("project name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if input.ID == "" {
		if id, ok, err := s.findProjectIDByNameLocked(input.Name); err != nil {
			return Project{}, err
		} else if ok {
			input.ID = id
		}
	}
	if input.ID == "" {
		id, err := s.nextProjectIDLocked(input.Name)
		if err != nil {
			return Project{}, err
		}
		input.ID = id
	}
	if !projectIDPattern.MatchString(input.ID) {
		return Project{}, fmt.Errorf("project id must match [a-zA-Z0-9_-]+")
	}

	now := time.Now().UTC()
	existing, found, err := s.readProjectLocked(input.ID)
	if err != nil {
		return Project{}, err
	}
	if found {
		input.CreatedAt = existing.CreatedAt
	} else {
		input.CreatedAt = now
	}
	input.UpdatedAt = now

	data, err := json.Marshal(input)
	if err != nil {
		return Project{}, fmt.Errorf("encode project: %w", err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketProjects))
		if b == nil {
			return fmt.Errorf("project db schema missing")
		}
		return b.Put([]byte(input.ID), data)
	}); err != nil {
		return Project{}, err
	}
	return input, nil
}

func (s *Store) bootstrap() error {
	if s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketProjects))
		if err != nil {
			return fmt.Errorf("init project db schema: %w", err)
		}
		return nil
	})
}

func (s *Store) listProjectsLocked() ([]Project, error) {
	if s.db == nil {
		return nil, nil
	}
	items := make([]Project, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketProjects))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			if len(k) == 0 || len(v) == 0 {
				return nil
			}
			var item Project
			if err := json.Unmarshal(v, &item); err != nil {
				return nil
			}
			item = normalizeProject(item)
			if item.ID == "" || item.Name == "" {
				return nil
			}
			items = append(items, item)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Store) readProjectLocked(id string) (Project, bool, error) {
	if s.db == nil {
		return Project{}, false, nil
	}

	item := Project{}
	found := false
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketProjects))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return Project{}, false, err
	}
	if !found {
		return Project{}, false, nil
	}
	return normalizeProject(item), true, nil
}

func (s *Store) findProjectIDByNameLocked(name string) (string, bool, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if normalizedName == "" {
		return "", false, nil
	}
	items, err := s.listProjectsLocked()
	if err != nil {
		return "", false, err
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item.Name)) == normalizedName {
			return item.ID, true, nil
		}
	}
	return "", false, nil
}

func (s *Store) nextProjectIDLocked(name string) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("project db unavailable")
	}

	base := sanitizeIdentifier(name)
	if base == "" {
		base = "project"
	}

	var next uint64
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketProjects))
		if b == nil {
			return fmt.Errorf("project db schema missing")
		}
		n, err := b.NextSequence()
		if err != nil {
			return err
		}
		next = n
		return nil
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", base, next), nil
}

func normalizeProject(input Project) Project {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = trimText(input.Name, 80)
	input.Goal = trimText(input.Goal, 220)
	input.Status = trimText(input.Status, 80)
	input.Summary = trimText(input.Summary, 360)
	input.KeyFacts = normalizeList(input.KeyFacts)
	input.Milestones = normalizeList(input.Milestones)
	input.Risks = normalizeList(input.Risks)
	input.Todos = normalizeList(input.Todos)
	input.Decisions = normalizeList(input.Decisions)
	return input
}

func normalizeList(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, 0, minInt(len(input), maxListItemsPerType))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		v := trimText(item, maxItemRunes)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
		if len(out) >= maxListItemsPerType {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimText(input string, maxRunes int) string {
	input = strings.ReplaceAll(strings.TrimSpace(input), "\n", " ")
	input = strings.Join(strings.Fields(input), " ")
	if maxRunes <= 0 || input == "" {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}

func sanitizeIdentifier(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteRune('-')
			lastDash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return ""
	}
	return id
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
