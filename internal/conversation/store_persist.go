package conversation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

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
