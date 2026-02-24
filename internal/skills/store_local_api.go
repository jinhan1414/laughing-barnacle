package skills

import (
	"strings"

	"laughing-barnacle/internal/localapi"
)

const legacyLocalAPIBaseURL = "http://127.0.0.1:8080"

func (s *Store) SetLocalAPIBaseURL(baseURL string) error {
	normalized := strings.TrimRight(localapi.ResolveBaseURL(baseURL), "/")

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localAPI == normalized {
		return nil
	}
	s.localAPI = normalized
	return s.ensureBuiltinSkillsLocked()
}

func (s *Store) localAPIBaseURLLocked() string {
	baseURL := strings.TrimSpace(s.localAPI)
	if baseURL == "" {
		return localapi.DefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func (s *Store) withLocalAPIBaseURLLocked(skill Skill) Skill {
	skill.Prompt = strings.ReplaceAll(
		skill.Prompt,
		legacyLocalAPIBaseURL,
		s.localAPIBaseURLLocked(),
	)
	return skill
}
