package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillResolver loads Skill definitions from disk + injected payloads.
//
// Skills are Markdown files with YAML frontmatter describing their name,
// description, and tool references. P0 (W10) supports two layouts:
//
//   <skill-dir>/<skill-name>/SKILL.md
//   <skill-dir>/<skill-name>.md
//
// The resolver returns the Skill's content (the Markdown body) so the
// executor can inject it into the agent's prompt before spawning.
type SkillResolver struct {
	// SkillDirs are searched in order. The first SKILL.md found wins.
	SkillDirs []string
}

// NewSkillResolver constructs a resolver with the default ~/.harnessx/skills
// directory plus any extras supplied.
func NewSkillResolver(extraDirs ...string) *SkillResolver {
	dirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".harnessx", "skills"))
	}
	dirs = append(dirs, extraDirs...)
	return &SkillResolver{SkillDirs: dirs}
}

// Skill is the resolved view of a SKILL.md file.
type Skill struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// Load returns the Skill with the given name, searching SkillDirs in order.
// Returns ErrSkillNotFound when no SKILL.md exists.
func (r *SkillResolver) Load(ctx context.Context, name string) (*Skill, error) {
	for _, dir := range r.SkillDirs {
		for _, layout := range []string{
			filepath.Join(dir, name, "SKILL.md"),
			filepath.Join(dir, name+".md"),
		} {
			raw, err := os.ReadFile(layout)
			if err != nil {
				continue
			}
			return parseSkill(raw, layout)
		}
	}
	return nil, ErrSkillNotFound
}

// List returns all Skill names reachable from SkillDirs.
func (r *SkillResolver) List(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, dir := range r.SkillDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err == nil {
				if !seen[e.Name()] {
					seen[e.Name()] = true
					out = append(out, e.Name())
				}
			}
		}
	}
	return out, nil
}

// ErrSkillNotFound is returned when Load cannot find a SKILL.md.
var ErrSkillNotFound = fmt.Errorf("skill not found")

// parseSkill splits YAML frontmatter from the Markdown body. Frontmatter
// ends at the first `---` line; everything after is the body.
func parseSkill(raw []byte, path string) (*Skill, error) {
	text := string(raw)
	if !strings.HasPrefix(text, "---") {
		return &Skill{Name: deriveNameFromPath(path), Body: text, Path: path}, nil
	}
	parts := strings.SplitN(text[3:], "---", 2)
	if len(parts) < 2 {
		return &Skill{Name: deriveNameFromPath(path), Body: text, Path: path}, nil
	}
	fm := parts[0]
	body := parts[1]
	skill := &Skill{
		Name: deriveNameFromPath(path),
		Body: strings.TrimSpace(body),
		Path: path,
	}
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "description:") {
			skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return skill, nil
}

func deriveNameFromPath(path string) string {
	base := filepath.Base(filepath.Dir(path))
	if base == "." || base == "/" {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return base
}
