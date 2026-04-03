package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nousresearch/hermes-go/pkg/eventbus"
	"github.com/nousresearch/hermes-go/pkg/memory"
	"github.com/nousresearch/hermes-go/pkg/skill"
)

type LearningLoop struct {
	mu          sync.Mutex
	episodic    *memory.EpisodicStore
	semantic    *memory.SemanticStore
	skillGen    *skill.Generator
	skillReg    *skill.Registry
	bus         *eventbus.Bus
	failures    []TaskOutcome
	maxFailures int
	storeDir    string
}

type TaskOutcome struct {
	Goal      string
	Summary   string
	Steps     []string
	ToolsUsed []string
	Success   bool
	Error     string
	Duration  time.Duration
	Timestamp time.Time
}

func NewLearningLoop(episodic *memory.EpisodicStore, semantic *memory.SemanticStore,
	skillGen *skill.Generator, skillReg *skill.Registry,
	bus *eventbus.Bus, storeDir string) *LearningLoop {

	return &LearningLoop{
		episodic:    episodic,
		semantic:    semantic,
		skillGen:    skillGen,
		skillReg:    skillReg,
		bus:         bus,
		failures:    make([]TaskOutcome, 0),
		maxFailures: 50,
		storeDir:    storeDir,
	}
}

func (ll *LearningLoop) RecordSuccess(ctx context.Context, outcome TaskOutcome) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	record := memory.EpisodicRecord{
		Goal:      outcome.Goal,
		Summary:   outcome.Summary,
		ToolsUsed: outcome.ToolsUsed,
		Success:   true,
		Duration:  outcome.Duration,
		Tags:      autoTag(outcome),
		CreatedAt: outcome.Timestamp,
	}
	_ = ll.episodic.Store(record)

	taskOutcome := skill.TaskOutcome{
		Goal:      outcome.Goal,
		Summary:   outcome.Summary,
		Steps:     outcome.Steps,
		ToolsUsed: outcome.ToolsUsed,
		Success:   true,
		Duration:  outcome.Duration,
	}

	if ll.skillGen.ShouldGenerate(taskOutcome) {
		skillObj, err := ll.skillGen.Generate(taskOutcome)
		if err == nil {
			_ = ll.skillReg.Save(*skillObj)
			ll.bus.Publish(eventbus.SkillCreated{
				SkillName: skillObj.Name,
				Timestamp: time.Now(),
			})
		}
	}

	facts := extractFacts(outcome)
	for _, f := range facts {
		_ = ll.semantic.Store(f)
	}
}

func (ll *LearningLoop) RecordFailure(ctx context.Context, outcome TaskOutcome) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	ll.failures = append(ll.failures, outcome)
	if len(ll.failures) > ll.maxFailures {
		ll.failures = ll.failures[1:]
	}

	record := memory.EpisodicRecord{
		Goal:      outcome.Goal,
		Summary:   outcome.Error,
		ToolsUsed: outcome.ToolsUsed,
		Success:   false,
		Duration:  outcome.Duration,
		Tags:      append(autoTag(outcome), "failed"),
		CreatedAt: outcome.Timestamp,
	}
	_ = ll.episodic.Store(record)

	_ = ll.saveFailures()
}

func (ll *LearningLoop) AnalyzeFailures() []Insight {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if len(ll.failures) == 0 {
		return nil
	}

	goalFailures := make(map[string]int)
	for _, f := range ll.failures {
		goalFailures[f.Goal]++
	}

	var insights []Insight
	for goal, count := range goalFailures {
		if count >= 3 {
			insights = append(insights, Insight{
				Type:        "recurring_failure",
				Description: fmt.Sprintf("Task '%s' has failed %d times", goal, count),
				Suggestion:  fmt.Sprintf("Consider creating a skill or reviewing the approach for: %s", goal),
				Confidence:  float64(count) / float64(len(ll.failures)),
			})
		}
	}

	return insights
}

func (ll *LearningLoop) GetFailures() []TaskOutcome {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	result := make([]TaskOutcome, len(ll.failures))
	copy(result, ll.failures)
	return result
}

type Insight struct {
	Type        string
	Description string
	Suggestion  string
	Confidence  float64
}

func autoTag(outcome TaskOutcome) []string {
	tags := make(map[string]bool)
	words := stringsFields(strings.ToLower(outcome.Goal))
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

func stringsFields(s string) []string {
	var result []string
	var current string
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func extractFacts(outcome TaskOutcome) []memory.SemanticFact {
	var facts []memory.SemanticFact

	if outcome.Success && len(outcome.ToolsUsed) > 0 {
		facts = append(facts, memory.SemanticFact{
			Category:   "pattern",
			Key:        fmt.Sprintf("successful_approach_%s", outcome.Goal),
			Value:      fmt.Sprintf("Used tools: %s", outcome.ToolsUsed),
			Tags:       outcome.ToolsUsed,
			Confidence: 0.6,
			Source:     "learned",
		})
	}

	return facts
}

func (ll *LearningLoop) saveFailures() error {
	if err := os.MkdirAll(ll.storeDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ll.failures, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(ll.storeDir, "failures.json")
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (ll *LearningLoop) loadFailures() error {
	path := filepath.Join(ll.storeDir, "failures.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &ll.failures)
}
