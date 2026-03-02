package skills

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/localapi"
)

const (
	autoSkillIDPrefix       = "auto-skill-"
	maxAutoSkillsRetained   = 24
	maxAutoSkillNameRunes   = 24
	maxAutoSkillPromptRunes = 180
	builtinSkillSource      = "builtin"
)

var (
	skillsSHSearchEndpoint        = "https://skills.sh/api/search"
	githubArchiveEndpointTemplate = "https://codeload.github.com/%s/%s/tar.gz/refs/heads/%s"
	gitCommandContext             = exec.CommandContext
	gitCloneRetryDelay            = 800 * time.Millisecond
	gitCloneMaxAttempts           = 2
)

type Skill struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	Enabled     bool
	Source      string
	UpdatedAt   time.Time
}

type CatalogSkill struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	SkillID  string `json:"skill_id"`
	Name     string `json:"name"`
	Installs int    `json:"installs"`
	URL      string `json:"url"`
}

type skillStateRecord struct {
	Enabled   bool      `json:"enabled"`
	Source    string    `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type stateFile struct {
	Skills map[string]skillStateRecord `json:"skills"`
}

type Store struct {
	dir        string
	statePath  string
	builtinDir string
	localAPI   string

	mu    sync.RWMutex
	state stateFile
}

func NewStore(dir, statePath string) (*Store, error) {
	return NewStoreWithBuiltinDir(dir, statePath, defaultBuiltinSkillsDir())
}

func NewStoreWithBuiltinDir(dir, statePath, builtinDir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	statePath = strings.TrimSpace(statePath)
	builtinDir = strings.TrimSpace(builtinDir)
	if dir == "" {
		return nil, fmt.Errorf("skills directory is required")
	}
	if statePath == "" {
		return nil, fmt.Errorf("skills state file path is required")
	}
	if builtinDir == "" {
		return nil, fmt.Errorf("builtin skills directory is required")
	}

	s := &Store{
		dir:        dir,
		statePath:  statePath,
		builtinDir: builtinDir,
		localAPI:   localapi.DefaultBaseURL,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
