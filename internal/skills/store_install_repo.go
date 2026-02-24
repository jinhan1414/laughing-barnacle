package skills

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) installFromRepo(ctx context.Context, repoURL, skillID, source string) (Skill, error) {
	repoURL = strings.TrimSpace(repoURL)
	skillID = sanitizeIdentifier(skillID)
	if repoURL == "" || skillID == "" {
		return Skill{}, fmt.Errorf("repo url and skill id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tmpRoot, err := os.MkdirTemp("", "skills-install-*")
	if err != nil {
		return Skill{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	repoPath := filepath.Join(tmpRoot, "repo")
	if err := fetchRepoSnapshot(ctx, repoURL, repoPath); err != nil {
		return Skill{}, fmt.Errorf("fetch repository snapshot failed: %w", err)
	}

	srcDir, err := findSkillDir(repoPath, skillID)
	if err != nil {
		return Skill{}, err
	}
	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); err != nil {
		return Skill{}, fmt.Errorf("skill file not found in repo: %w", err)
	}

	dstDir := filepath.Join(s.dir, skillID)
	if err := os.RemoveAll(dstDir); err != nil {
		return Skill{}, fmt.Errorf("clear existing skill dir: %w", err)
	}
	if err := copyDir(srcDir, dstDir); err != nil {
		return Skill{}, err
	}

	record := s.state.Skills[skillID]
	record.Enabled = true
	record.Source = strings.TrimSpace(source)
	record.UpdatedAt = time.Now()
	s.state.Skills[skillID] = record
	if err := s.persistLocked(); err != nil {
		return Skill{}, err
	}

	skills, err := s.listSkillsLocked()
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if skill.ID == skillID {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("installed skill %q not found", skillID)
}

func fetchRepoSnapshot(ctx context.Context, repoURL, repoPath string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return fmt.Errorf("repo url is required")
	}
	if repoPath == "" {
		return fmt.Errorf("repo path is required")
	}
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("clear temp repo path: %w", err)
	}

	cloneErr := cloneRepoDepth1(ctx, repoURL, repoPath)
	if cloneErr == nil {
		return nil
	}

	owner, repo, ok := parseGitHubOwnerRepo(repoURL)
	if !ok {
		return fmt.Errorf("clone failed and repo is not a github.com url: %w", cloneErr)
	}

	var archiveErr error
	for _, branch := range []string{"main", "master"} {
		if err := os.RemoveAll(repoPath); err != nil {
			return fmt.Errorf("clear temp repo path before archive download: %w", err)
		}
		if err := downloadAndExtractGitHubArchive(ctx, owner, repo, branch, repoPath); err == nil {
			return nil
		} else {
			archiveErr = err
		}
	}

	if archiveErr != nil {
		return fmt.Errorf("clone error: %v; archive fallback error: %w", cloneErr, archiveErr)
	}
	return cloneErr
}

func cloneRepoDepth1(ctx context.Context, repoURL, repoPath string) error {
	attempts := gitCloneMaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		cmd := gitCommandContext(ctx, "git", "clone", "--depth", "1", repoURL, repoPath)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("git clone attempt %d/%d failed: %v (%s)", attempt, attempts, err, strings.TrimSpace(string(out)))

		if attempt >= attempts || ctx.Err() != nil {
			break
		}
		_ = os.RemoveAll(repoPath)
		timer := time.NewTimer(gitCloneRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
	return lastErr
}

func parseGitHubOwnerRepo(repoURL string) (owner, repo string, ok bool) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", "", false
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	parts := splitPathSegments(parsed.Path)
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

func downloadAndExtractGitHubArchive(ctx context.Context, owner, repo, branch, repoPath string) error {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	branch = strings.TrimSpace(branch)
	if owner == "" || repo == "" || branch == "" {
		return fmt.Errorf("owner/repo/branch are required")
	}

	archiveURL := fmt.Sprintf(githubArchiveEndpointTemplate, owner, repo, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return fmt.Errorf("build archive request: %w", err)
	}
	resp, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("archive http status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo path: %w", err)
	}
	if err := extractGitHubTarGz(resp.Body, repoPath); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}
	return nil
}

func extractGitHubTarGz(src io.Reader, dst string) error {
	gzipReader, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	dst = filepath.Clean(dst)
	tr := tar.NewReader(gzipReader)
	for {
		header, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		cleaned := filepath.Clean(name)
		parts := strings.Split(cleaned, "/")
		if len(parts) < 2 {
			continue
		}
		rel := filepath.Join(parts[1:]...)
		target := filepath.Join(dst, rel)
		target = filepath.Clean(target)
		if target != dst && !strings.HasPrefix(target, dst+string(filepath.Separator)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Chmod(0o600); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
