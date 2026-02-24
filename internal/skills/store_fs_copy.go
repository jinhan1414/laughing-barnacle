package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func findSkillDir(repoPath, skillID string) (string, error) {
	candidates := []string{
		filepath.Join(repoPath, "skills", skillID),
		filepath.Join(repoPath, skillID),
	}
	for _, dir := range candidates {
		if stat, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil && !stat.IsDir() {
			return dir, nil
		}
	}

	best := ""
	bestDepth := 1 << 30
	walkErr := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if filepath.Base(path) != skillID {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return nil
		}
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth < bestDepth {
			bestDepth = depth
			best = path
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("scan repo skills: %w", walkErr)
	}
	if best == "" {
		return "", fmt.Errorf("skill %q not found in repository", skillID)
	}
	return best, nil
}

func copyDir(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination skill dir: %w", err)
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination file dir: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	if err := out.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod destination file: %w", err)
	}
	return nil
}

func cloneSkills(in []Skill) []Skill {
	if len(in) == 0 {
		return nil
	}
	out := make([]Skill, len(in))
	copy(out, in)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
