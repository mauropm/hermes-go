package proactive

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nousresearch/hermes-go/pkg/memory"
)

type PatternDetector struct {
	memory *memory.EpisodicStore
}

type Pattern struct {
	Type        string
	Description string
	Confidence  float64
	Message     string
	Reason      string
}

func NewPatternDetector(episodic *memory.EpisodicStore) *PatternDetector {
	return &PatternDetector{memory: episodic}
}

func (pd *PatternDetector) Detect(ctx context.Context) []Pattern {
	var patterns []Pattern
	patterns = append(patterns, pd.detectTimePatterns(ctx)...)
	patterns = append(patterns, pd.detectFrequencyPatterns(ctx)...)
	patterns = append(patterns, pd.detectSequencePatterns(ctx)...)
	return patterns
}

func (pd *PatternDetector) detectTimePatterns(ctx context.Context) []Pattern {
	now := time.Now()
	records, err := pd.memory.Recent(now.AddDate(0, 0, -7), 100)
	if err != nil || len(records) == 0 {
		return nil
	}

	hourGoalCounts := make(map[string]map[int]int)
	for _, r := range records {
		h := r.CreatedAt.Hour()
		if hourGoalCounts[r.Goal] == nil {
			hourGoalCounts[r.Goal] = make(map[int]int)
		}
		hourGoalCounts[r.Goal][h]++
	}

	var patterns []Pattern
	currentHour := now.Hour()

	for goal, hours := range hourGoalCounts {
		count := hours[currentHour]
		total := 0
		for _, c := range hours {
			total += c
		}

		if count >= 2 && total >= 3 {
			confidence := float64(count) / float64(total)
			patterns = append(patterns, Pattern{
				Type:       "time_based",
				Confidence: confidence,
				Message:    fmt.Sprintf("You usually %s around this time", goal),
				Reason:     fmt.Sprintf("Done %d of %d times at hour %d this week", count, total, currentHour),
			})
		}
	}

	return patterns
}

func (pd *PatternDetector) detectFrequencyPatterns(ctx context.Context) []Pattern {
	now := time.Now()
	records, err := pd.memory.Recent(now.AddDate(0, 0, -7), 100)
	if err != nil || len(records) == 0 {
		return nil
	}

	goalCounts := make(map[string]int)
	for _, r := range records {
		goalCounts[r.Goal]++
	}

	var patterns []Pattern
	for goal, count := range goalCounts {
		if count >= 5 {
			patterns = append(patterns, Pattern{
				Type:       "frequency",
				Confidence: float64(count) / 7.0,
				Message:    fmt.Sprintf("You've been working on %s frequently (%d times this week)", goal, count),
				Reason:     fmt.Sprintf("High frequency: %d occurrences in 7 days", count),
			})
		}
	}

	return patterns
}

func (pd *PatternDetector) detectSequencePatterns(ctx context.Context) []Pattern {
	now := time.Now()
	records, err := pd.memory.Recent(now.AddDate(0, 0, -1), 50)
	if err != nil || len(records) < 3 {
		return nil
	}

	var patterns []Pattern

	goalTransitions := make(map[string]map[string]int)
	for i := 0; i < len(records)-1; i++ {
		from := records[i].Goal
		to := records[i+1].Goal
		if from == to {
			continue
		}
		if goalTransitions[from] == nil {
			goalTransitions[from] = make(map[string]int)
		}
		goalTransitions[from][to]++
	}

	for from, transitions := range goalTransitions {
		for to, count := range transitions {
			if count >= 2 {
				patterns = append(patterns, Pattern{
					Type:       "sequence",
					Confidence: float64(count) * 0.3,
					Message:    fmt.Sprintf("After %s, you often %s", from, to),
					Reason:     fmt.Sprintf("Observed %d transitions from '%s' to '%s'", count, from, to),
				})
			}
		}
	}

	return patterns
}

func normalizeGoal(goal string) string {
	goal = strings.ToLower(goal)
	goal = strings.TrimSpace(goal)
	if len(goal) > 100 {
		goal = goal[:100]
	}
	return goal
}
