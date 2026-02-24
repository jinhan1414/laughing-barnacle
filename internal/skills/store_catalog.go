package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Store) InstallFromSkillsSH(ctx context.Context, rawURL string) (Skill, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Skill{}, fmt.Errorf("skills.sh url is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Skill{}, fmt.Errorf("invalid skills.sh url: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "skills.sh" && host != "www.skills.sh" {
		return Skill{}, fmt.Errorf("url host must be skills.sh")
	}

	segments := splitPathSegments(parsed.Path)
	if len(segments) < 3 {
		return Skill{}, fmt.Errorf("skills.sh url must be /{owner}/{repo}/{skill}")
	}
	owner := segments[0]
	repo := segments[1]
	skillID := sanitizeIdentifier(segments[2])
	if skillID == "" {
		return Skill{}, fmt.Errorf("invalid skill id from url")
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	return s.installFromRepo(ctx, repoURL, skillID, rawURL)
}

func (s *Store) SearchSkillsCatalog(ctx context.Context, query string, limit int) ([]CatalogSkill, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 30 {
		limit = 30
	}

	reqURL := skillsSHSearchEndpoint + "?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build search request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("search skills.sh: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("skills.sh search failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Skills []struct {
			ID       string `json:"id"`
			Source   string `json:"source"`
			SkillID  string `json:"skillId"`
			Name     string `json:"name"`
			Installs int    `json:"installs"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode skills.sh search response: %w", err)
	}

	out := make([]CatalogSkill, 0, minInt(limit, len(payload.Skills)))
	for _, item := range payload.Skills {
		source := strings.TrimSpace(item.Source)
		skillID := strings.TrimSpace(item.SkillID)
		if source == "" && strings.TrimSpace(item.ID) != "" {
			parts := splitPathSegments(item.ID)
			if len(parts) >= 3 {
				source = strings.TrimSpace(parts[0] + "/" + parts[1])
				if skillID == "" {
					skillID = strings.TrimSpace(parts[2])
				}
			}
		}
		if source == "" || skillID == "" {
			continue
		}
		out = append(out, CatalogSkill{
			ID:       strings.TrimSpace(item.ID),
			Source:   source,
			SkillID:  skillID,
			Name:     strings.TrimSpace(item.Name),
			Installs: item.Installs,
			URL:      fmt.Sprintf("https://skills.sh/%s/%s", source, skillID),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
