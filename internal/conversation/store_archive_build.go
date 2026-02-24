package conversation

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
