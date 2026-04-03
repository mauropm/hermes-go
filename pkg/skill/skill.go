package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name          string    `yaml:"name" json:"name"`
	Description   string    `yaml:"description" json:"description"`
	Version       string    `yaml:"version" json:"version"`
	Tags          []string  `yaml:"tags" json:"tags"`
	ToolsRequired []string  `yaml:"tools_required" json:"tools_required"`
	Confidence    float64   `yaml:"confidence" json:"confidence"`
	CreatedAt     time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt     time.Time `yaml:"updated_at" json:"updated_at"`
	UsageCount    int       `yaml:"usage_count" json:"usage_count"`
	SuccessRate   float64   `yaml:"success_rate" json:"success_rate"`
	Content       string    `yaml:"-" json:"-"`
	FilePath      string    `yaml:"-" json:"-"`
}

type Registry struct {
	mu      sync.RWMutex
	skills  map[string]*Skill
	homeDir string
}

func NewRegistry(homeDir string) *Registry {
	return &Registry{
		skills:  make(map[string]*Skill),
		homeDir: homeDir,
	}
}

func (r *Registry) LoadAll() error {
	skillsDir := filepath.Join(r.homeDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return fmt.Errorf("read skills dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if err := r.Load(skillPath); err != nil {
				continue
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			skillPath := filepath.Join(skillsDir, entry.Name())
			if err := r.Load(skillPath); err != nil {
				continue
			}
		}
	}

	return nil
}

func (r *Registry) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read skill file: %w", err)
	}

	content := string(data)
	frontmatter, body := parseFrontmatter(content)

	var skill Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
		return fmt.Errorf("parse skill frontmatter: %w", err)
	}

	skill.Content = strings.TrimSpace(body)
	skill.FilePath = path

	if skill.Name == "" {
		skill.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if skill.CreatedAt.IsZero() {
		skill.CreatedAt = time.Now()
	}
	if skill.UpdatedAt.IsZero() {
		skill.UpdatedAt = time.Now()
	}

	r.mu.Lock()
	r.skills[skill.Name] = &skill
	r.mu.Unlock()

	return nil
}

func (r *Registry) Match(query string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryTerms := strings.Fields(strings.ToLower(query))
	scored := make([]scoredSkill, 0, len(r.skills))

	for _, skill := range r.skills {
		score := skillRelevance(skill, queryTerms)
		if score > 0 {
			scored = append(scored, scoredSkill{skill: skill, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	results := make([]Skill, len(scored))
	for i, s := range scored {
		results[i] = *s.skill
	}
	return results
}

func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

func (r *Registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		results = append(results, *s)
	}
	return results
}

func (r *Registry) RecordUsage(name string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.skills[name]
	if !ok {
		return
	}

	s.UsageCount++
	if s.UsageCount == 1 {
		s.SuccessRate = 1.0
	} else {
		s.SuccessRate = (s.SuccessRate*float64(s.UsageCount-1) + boolToFloat(success)) / float64(s.UsageCount)
	}
	s.UpdatedAt = time.Now()
}

func (r *Registry) Save(skill Skill) error {
	if skill.Name == "" {
		return fmt.Errorf("skill name cannot be empty")
	}

	skillsDir := filepath.Join(r.homeDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	frontmatter, err := yaml.Marshal(skill)
	if err != nil {
		return fmt.Errorf("marshal skill: %w", err)
	}

	content := fmt.Sprintf("---\n%s---\n\n%s\n", string(frontmatter), skill.Content)

	filePath := filepath.Join(skillsDir, skill.Name+".md")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	skill.FilePath = filePath
	skill.UpdatedAt = time.Now()

	r.mu.Lock()
	r.skills[skill.Name] = &skill
	r.mu.Unlock()

	return nil
}

func (r *Registry) FormatForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")
	b.WriteString("You have the following skills available. Reference them when relevant.\n\n")

	for _, s := range skills {
		b.WriteString(fmt.Sprintf("## %s\n", s.Name))
		b.WriteString(fmt.Sprintf("Description: %s\n", s.Description))
		b.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(s.Tags, ", ")))
		b.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", s.Confidence*100))
		b.WriteString("\n")
	}

	b.WriteString("</available_skills>\n")
	return b.String()
}

type scoredSkill struct {
	skill *Skill
	score float64
}

func skillRelevance(skill *Skill, queryTerms []string) float64 {
	var score float64

	skillText := strings.ToLower(skill.Name + " " + skill.Description + " " + strings.Join(skill.Tags, " "))
	skillWords := strings.Fields(skillText)

	for _, qt := range queryTerms {
		for _, sw := range skillWords {
			if sw == qt {
				score += 1.0
			} else if strings.Contains(sw, qt) || strings.Contains(qt, sw) {
				score += 0.5
			}
		}
	}

	score *= skill.Confidence
	return score
}

func parseFrontmatter(content string) (string, string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", content
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) != 2 {
		return "", content
	}

	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

type SkillExport struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	Tags          []string `json:"tags"`
	ToolsRequired []string `json:"tools_required"`
	Confidence    float64  `json:"confidence"`
	Content       string   `json:"content"`
}

func (s *Skill) Export() SkillExport {
	return SkillExport{
		Name:          s.Name,
		Description:   s.Description,
		Version:       s.Version,
		Tags:          s.Tags,
		ToolsRequired: s.ToolsRequired,
		Confidence:    s.Confidence,
		Content:       s.Content,
	}
}

func (s *Skill) ToJSON() ([]byte, error) {
	return json.Marshal(s.Export())
}
