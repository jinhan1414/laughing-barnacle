package conversation

import (
	"fmt"
	"strings"
)

func (s *Store) ListSummaryArchiveIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, refs := splitSummaryArchiveRefs(s.summary)
	if len(refs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
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
	return archiveRecordToIndex(record), nil
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
			replayMessages := cloneArchiveReplayMessages(sec.Messages)
			legacyIncomplete := len(replayMessages) == 0
			return ArchiveSection{
				ID:               sec.ID,
				Title:            sec.Title,
				Digest:           sec.Digest,
				Content:          renderArchiveSectionContent(sec.Content, replayMessages),
				Messages:         replayMessages,
				LegacyIncomplete: legacyIncomplete,
			}, nil
		}
	}
	return ArchiveSection{}, ErrArchiveSectionNotFound
}

func archiveRecordToIndex(record archiveRecord) ArchiveIndex {
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
	}
}

func cloneArchiveReplayMessages(messages []ArchiveReplayMessage) []ArchiveReplayMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]ArchiveReplayMessage, len(messages))
	copy(out, messages)
	return out
}

func renderArchiveSectionContent(legacyContent string, messages []ArchiveReplayMessage) string {
	if len(messages) == 0 {
		return strings.TrimSpace(legacyContent)
	}
	lines := make([]string, 0, len(messages))
	for i, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "unknown"
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", i+1, role, strings.TrimSpace(msg.Content)))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
