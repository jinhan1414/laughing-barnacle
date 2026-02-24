package conversation

import (
	"strings"
)

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
