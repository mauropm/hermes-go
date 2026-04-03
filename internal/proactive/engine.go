package proactive

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nousresearch/hermes-go/pkg/eventbus"
	"github.com/nousresearch/hermes-go/pkg/memory"
)

type Engine struct {
	memory   *memory.EpisodicStore
	patterns *PatternDetector
	bus      *eventbus.Bus
	config   Config
}

type Config struct {
	Enabled               bool
	CheckInterval         time.Duration
	MinConfidence         float64
	QuietHoursStart       string
	QuietHoursEnd         string
	MaxSuggestionsPerHour int
}

func NewEngine(episodic *memory.EpisodicStore, bus *eventbus.Bus, config Config) *Engine {
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Minute
	}
	if config.MinConfidence == 0 {
		config.MinConfidence = 0.5
	}
	if config.QuietHoursStart == "" {
		config.QuietHoursStart = "22:00"
	}
	if config.QuietHoursEnd == "" {
		config.QuietHoursEnd = "07:00"
	}
	if config.MaxSuggestionsPerHour == 0 {
		config.MaxSuggestionsPerHour = 3
	}

	return &Engine{
		memory:   episodic,
		patterns: NewPatternDetector(episodic),
		bus:      bus,
		config:   config,
	}
}

func (e *Engine) Start(ctx context.Context) {
	if !e.config.Enabled {
		return
	}

	ticker := time.NewTicker(e.config.CheckInterval)
	defer ticker.Stop()

	suggestionCount := 0
	hourReset := time.Now().Truncate(time.Hour).Add(time.Hour)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			if now.After(hourReset) {
				suggestionCount = 0
				hourReset = now.Truncate(time.Hour).Add(time.Hour)
			}

			if e.isQuietHours(now) {
				continue
			}

			if suggestionCount >= e.config.MaxSuggestionsPerHour {
				continue
			}

			patterns := e.patterns.Detect(ctx)
			for _, p := range patterns {
				if p.Confidence >= e.config.MinConfidence {
					e.bus.Publish(eventbus.ProactiveSuggestion{
						Suggestion: p.Message,
						Reason:     p.Reason,
						Confidence: p.Confidence,
						Timestamp:  time.Now(),
					})
					suggestionCount++
					break
				}
			}
		}
	}
}

func (e *Engine) isQuietHours(now time.Time) bool {
	start := parseTime(e.config.QuietHoursStart)
	end := parseTime(e.config.QuietHoursEnd)

	current := now.Hour()*60 + now.Minute()

	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func parseTime(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0
	}
	h := 0
	m := 0
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return h*60 + m
}
