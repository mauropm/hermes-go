package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nousresearch/hermes-go/pkg/eventbus"
)

type Scheduler struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	ticker   *time.Ticker
	bus      *eventbus.Bus
	storeDir string
	running  bool
	stopCh   chan struct{}
}

type Job struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Prompt     string         `json:"prompt"`
	Schedule   Schedule       `json:"schedule"`
	Enabled    bool           `json:"enabled"`
	State      JobState       `json:"state"`
	Deliver    DeliveryTarget `json:"deliver"`
	Skills     []string       `json:"skills,omitempty"`
	Toolsets   []string       `json:"toolsets,omitempty"`
	MaxTurns   int            `json:"max_turns,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	NextRunAt  time.Time      `json:"next_run_at"`
	LastRunAt  time.Time      `json:"last_run_at,omitempty"`
	LastStatus string         `json:"last_status,omitempty"`
	LastError  string         `json:"last_error,omitempty"`
	RunCount   int            `json:"run_count"`
}

type Schedule struct {
	Kind       string `json:"kind"`
	Expression string `json:"expression"`
}

type DeliveryTarget struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

type JobState string

const (
	StateScheduled JobState = "scheduled"
	StateRunning   JobState = "running"
	StatePaused    JobState = "paused"
	StateCompleted JobState = "completed"
	StateFailed    JobState = "failed"
)

type JobExecutor func(ctx context.Context, job *Job) error

func NewScheduler(storeDir string, bus *eventbus.Bus) *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]*Job),
		storeDir: storeDir,
		bus:      bus,
		ticker:   time.NewTicker(60 * time.Second),
		stopCh:   make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context, executor JobExecutor) {
	s.running = true

	if err := s.loadJobs(); err != nil {
		fmt.Fprintf(os.Stderr, "[scheduler] failed to load jobs: %v\n", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.ticker.Stop()
				s.running = false
				return
			case <-s.stopCh:
				s.ticker.Stop()
				s.running = false
				return
			case <-s.ticker.C:
				s.tick(ctx, executor)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	if s.running {
		close(s.stopCh)
	}
}

func (s *Scheduler) AddJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		job.ID = uuid.New().String()[:12]
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.State == "" {
		job.State = StateScheduled
	}
	if !job.Enabled {
		job.State = StatePaused
	}

	job.NextRunAt = job.Schedule.Next(time.Now())

	s.jobs[job.ID] = job
	return s.saveJobs()
}

func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jobs, id)
	return s.saveJobs()
}

func (s *Scheduler) PauseJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	job.State = StatePaused
	return s.saveJobs()
}

func (s *Scheduler) ResumeJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	job.State = StateScheduled
	job.NextRunAt = job.Schedule.Next(time.Now())
	return s.saveJobs()
}

func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, job)
	}
	return result
}

func (s *Scheduler) GetJob(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *Scheduler) tick(ctx context.Context, executor JobExecutor) {
	s.mu.RLock()
	var due []*Job
	now := time.Now()
	for _, job := range s.jobs {
		if job.Enabled && job.State == StateScheduled && !job.NextRunAt.After(now) {
			due = append(due, job)
		}
	}
	s.mu.RUnlock()

	for _, job := range due {
		s.mu.Lock()
		job.NextRunAt = job.Schedule.Next(now)
		job.State = StateRunning
		s.mu.Unlock()

		go s.runJob(ctx, job, executor)
	}
}

func (s *Scheduler) runJob(ctx context.Context, job *Job, executor JobExecutor) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	start := time.Now()
	err := executor(ctx, job)

	s.mu.Lock()
	job.LastRunAt = start
	job.RunCount++
	if err != nil {
		job.LastStatus = "failed"
		job.LastError = err.Error()
		job.State = StateScheduled
	} else {
		job.LastStatus = "completed"
		job.State = StateScheduled
	}
	s.mu.Unlock()

	_ = s.saveJobs()

	if s.bus != nil {
		s.bus.Publish(eventbus.JobCompleted{
			JobID:     job.ID,
			Success:   err == nil,
			Timestamp: time.Now(),
		})
	}
}

func (s *Scheduler) saveJobs() error {
	data, err := json.MarshalIndent(s.jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal jobs: %w", err)
	}

	if err := os.MkdirAll(s.storeDir, 0o755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}

	path := filepath.Join(s.storeDir, "jobs.json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	return os.Rename(tmpPath, path)
}

func (s *Scheduler) loadJobs() error {
	path := filepath.Join(s.storeDir, "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.jobs)
}

func (s Schedule) Next(from time.Time) time.Time {
	switch s.Kind {
	case "cron":
		return nextCron(s.Expression, from)
	case "interval":
		d, err := time.ParseDuration(s.Expression)
		if err != nil {
			return from.Add(24 * time.Hour)
		}
		return from.Add(d)
	case "once":
		t, err := time.Parse(time.RFC3339, s.Expression)
		if err != nil {
			return from.Add(24 * time.Hour)
		}
		return t
	default:
		return from.Add(24 * time.Hour)
	}
}

func nextCron(expr string, from time.Time) time.Time {
	parts := parseCron(expr)
	if parts == nil {
		return from.Add(24 * time.Hour)
	}

	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525600; i++ {
		if matchesCron(t, parts) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return from.Add(24 * time.Hour)
}

type cronParts struct {
	minute    map[int]bool
	hour      map[int]bool
	dayMonth  map[int]bool
	month     map[int]bool
	dayOfWeek map[int]bool
}

func parseCron(expr string) *cronParts {
	fields := splitFields(expr)
	if len(fields) != 5 {
		return nil
	}

	return &cronParts{
		minute:    parseField(fields[0], 0, 59),
		hour:      parseField(fields[1], 0, 23),
		dayMonth:  parseField(fields[2], 1, 31),
		month:     parseField(fields[3], 1, 12),
		dayOfWeek: parseField(fields[4], 0, 6),
	}
}

func splitFields(expr string) []string {
	var fields []string
	var current string
	for _, r := range expr {
		if r == ' ' || r == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

func parseField(field string, min, max int) map[int]bool {
	result := make(map[int]bool)

	if field == "*" {
		for i := min; i <= max; i++ {
			result[i] = true
		}
		return result
	}

	for _, part := range splitByComma(field) {
		if part == "*" {
			for i := min; i <= max; i++ {
				result[i] = true
			}
			continue
		}

		if len(part) > 2 && part[1] == '/' {
			step := parseInt(part[2:])
			start := min
			if part[0] != '*' {
				start = parseInt(part[:1])
			}
			for i := start; i <= max; i += step {
				result[i] = true
			}
			continue
		}

		if len(part) > 2 {
			for i := 0; i < len(part); i++ {
				if part[i] == '-' {
					start := parseInt(part[:i])
					end := parseInt(part[i+1:])
					for j := start; j <= end; j++ {
						result[j] = true
					}
					break
				}
			}
		} else {
			result[parseInt(part)] = true
		}
	}

	return result
}

func splitByComma(s string) []string {
	var parts []string
	var current string
	for _, r := range s {
		if r == ',' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func parseInt(s string) int {
	var n int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}

func matchesCron(t time.Time, parts *cronParts) bool {
	if !parts.minute[t.Minute()] {
		return false
	}
	if !parts.hour[t.Hour()] {
		return false
	}
	if !parts.dayMonth[t.Day()] {
		return false
	}
	if !parts.month[int(t.Month())] {
		return false
	}
	if !parts.dayOfWeek[int(t.Weekday())] {
		return false
	}
	return true
}
