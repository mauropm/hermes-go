package skill

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Generator struct {
	skillsDir    string
	minSuccesses int
}

func NewGenerator(skillsDir string, minSuccesses int) *Generator {
	if minSuccesses <= 0 {
		minSuccesses = 3
	}
	return &Generator{
		skillsDir:    skillsDir,
		minSuccesses: minSuccesses,
	}
}

type TaskOutcome struct {
	Goal      string
	Summary   string
	Steps     []string
	ToolsUsed []string
	Success   bool
	Duration  time.Duration
}

func (g *Generator) ShouldGenerate(outcome TaskOutcome) bool {
	if !outcome.Success {
		return false
	}
	if len(outcome.ToolsUsed) == 0 {
		return false
	}
	return true
}

func (g *Generator) Generate(outcome TaskOutcome) (*Skill, error) {
	skill := &Skill{
		Name:          slugify(outcome.Goal),
		Description:   fmt.Sprintf("Auto-generated: %s", outcome.Goal),
		Version:       "1.0.0",
		Tags:          extractTags(outcome),
		ToolsRequired: outcome.ToolsUsed,
		Confidence:    0.5,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Content:       generateMarkdown(outcome),
	}

	return skill, nil
}

func generateMarkdown(outcome TaskOutcome) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", outcome.Goal))

	b.WriteString("## When to Use\n")
	b.WriteString(fmt.Sprintf("When the user asks to %s.\n\n", outcome.Goal))

	b.WriteString("## Steps\n")
	for i, step := range outcome.Steps {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}
	b.WriteString("\n")

	if len(outcome.ToolsUsed) > 0 {
		b.WriteString("## Tools Used\n")
		for _, t := range outcome.ToolsUsed {
			b.WriteString(fmt.Sprintf("- %s\n", t))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("## Summary\n%s\n", outcome.Summary))

	return b.String()
}

func extractTags(outcome TaskOutcome) []string {
	tags := make(map[string]bool)

	words := strings.Fields(strings.ToLower(outcome.Goal))
	for _, w := range words {
		if len(w) > 3 {
			tags[w] = true
		}
	}

	for _, t := range outcome.ToolsUsed {
		tags[t] = true
	}

	result := make([]string, 0, len(tags))
	for t := range tags {
		result = append(result, t)
	}
	return result
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	slug := result.String()
	if len(slug) > 50 {
		slug = slug[:50]
	}

	return fmt.Sprintf("%s-%s", slug, uuid.New().String()[:8])
}
