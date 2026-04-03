package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TrustLevel string

const (
	TrustTrusted   TrustLevel = "trusted"
	TrustInferred  TrustLevel = "inferred"
	TrustUntrusted TrustLevel = "untrusted"
)

type EpisodicRecord struct {
	ID        string        `json:"id"`
	Goal      string        `json:"goal"`
	Summary   string        `json:"summary"`
	Steps     []StepRecord  `json:"steps,omitempty"`
	ToolsUsed []string      `json:"tools_used"`
	Success   bool          `json:"success"`
	Duration  time.Duration `json:"duration"`
	Tags      []string      `json:"tags"`
	CreatedAt time.Time     `json:"created_at"`
	ExpiresAt time.Time     `json:"expires_at"`
	Hash      string        `json:"hash"`
	Size      int           `json:"size"`
}

type StepRecord struct {
	Description string `json:"description"`
	Tool        string `json:"tool,omitempty"`
	Success     bool   `json:"success"`
}

type EpisodicStore struct {
	mu       sync.RWMutex
	records  []EpisodicRecord
	index    *InvertedIndex
	storeDir string
	ttl      time.Duration
	maxSize  int
}

func NewEpisodicStore(homeDir string, ttl time.Duration) (*EpisodicStore, error) {
	storeDir := filepath.Join(homeDir, "memory", "episodic")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create episodic memory dir: %w", err)
	}

	s := &EpisodicStore{
		records:  make([]EpisodicRecord, 0),
		index:    NewInvertedIndex(),
		storeDir: storeDir,
		ttl:      ttl,
		maxSize:  10 * 1024 * 1024, // 10MB
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load episodic store: %w", err)
	}

	return s, nil
}

func (s *EpisodicStore) Store(record EpisodicRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(record.Summary) > 4096 {
		record.Summary = record.Summary[:4096]
	}

	hash := sha256.Sum256([]byte(record.Goal + record.Summary))
	record.Hash = hex.EncodeToString(hash[:])
	record.ID = fmt.Sprintf("ep_%x", time.Now().UnixNano())
	record.CreatedAt = time.Now()
	record.ExpiresAt = time.Now().Add(s.ttl)
	record.Size = len(record.Summary)

	totalSize := s.totalSizeLocked()
	if totalSize+record.Size > s.maxSize {
		s.evictOldestLocked()
	}

	s.records = append(s.records, record)
	s.index.Index(record.ID, append(record.Tags, strings.Fields(record.Goal)...))

	return s.persist()
}

func (s *EpisodicStore) RetrieveByTags(tags []string, limit int) ([]EpisodicRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []EpisodicRecord
	for _, r := range s.records {
		if recordHasTags(r, tags) {
			results = append(results, r)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *EpisodicStore) RetrieveByRelevance(query string, limit int) ([]EpisodicRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queryTerms := strings.Fields(strings.ToLower(query))
	scored := make([]scoredRecord, 0, len(s.records))

	for _, r := range s.records {
		score := s.index.RelevanceScore(queryTerms, append(r.Tags, strings.Fields(strings.ToLower(r.Goal))...))
		if score > 0 {
			scored = append(scored, scoredRecord{record: r, score: score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]EpisodicRecord, len(scored))
	for i, s := range scored {
		results[i] = s.record
	}
	return results, nil
}

func (s *EpisodicStore) Recent(since time.Time, limit int) ([]EpisodicRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []EpisodicRecord
	for _, r := range s.records {
		if r.CreatedAt.After(since) {
			results = append(results, r)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *EpisodicStore) Statistics() (StoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := StoreStats{
		TotalRecords: len(s.records),
		TotalSize:    s.totalSizeLocked(),
	}
	for _, r := range s.records {
		if r.Success {
			stats.SuccessCount++
		} else {
			stats.FailureCount++
		}
	}
	return stats, nil
}

func (s *EpisodicStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]EpisodicRecord, 0)
	s.index = NewInvertedIndex()
	return s.persist()
}

type StoreStats struct {
	TotalRecords int `json:"total_records"`
	SuccessCount int `json:"success_count"`
	FailureCount int `json:"failure_count"`
	TotalSize    int `json:"total_size"`
}

type scoredRecord struct {
	record EpisodicRecord
	score  float64
}

func recordHasTags(r EpisodicRecord, tags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range r.Tags {
		tagSet[t] = true
	}
	for _, t := range tags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func (s *EpisodicStore) totalSizeLocked() int {
	total := 0
	for _, r := range s.records {
		total += r.Size
	}
	return total
}

func (s *EpisodicStore) evictOldestLocked() {
	if len(s.records) == 0 {
		return
	}
	s.records = s.records[1:]
}

func (s *EpisodicStore) persist() error {
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal episodic records: %w", err)
	}

	path := filepath.Join(s.storeDir, "store.json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

func (s *EpisodicStore) load() error {
	path := filepath.Join(s.storeDir, "store.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &s.records); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	for _, r := range s.records {
		s.index.Index(r.ID, append(r.Tags, strings.Fields(strings.ToLower(r.Goal))...))
	}
	return nil
}

func FormatEpisodicContext(records []EpisodicRecord) string {
	if len(records) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<episodic_memory>\n")
	b.WriteString("Relevant past experiences:\n\n")

	for _, r := range records {
		status := "completed"
		if !r.Success {
			status = "failed"
		}
		b.WriteString(fmt.Sprintf("[%s | %s | %s]\n", r.Goal, status, r.CreatedAt.Format(time.RFC3339)))
		b.WriteString(r.Summary + "\n\n")
	}

	b.WriteString("</episodic_memory>\n")
	return b.String()
}
