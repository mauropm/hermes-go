package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SemanticFact struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Tags       []string  `json:"tags"`
	Confidence float64   `json:"confidence"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SemanticStore struct {
	mu       sync.RWMutex
	facts    map[string]SemanticFact
	index    *InvertedIndex
	storeDir string
}

func NewSemanticStore(homeDir string) (*SemanticStore, error) {
	storeDir := filepath.Join(homeDir, "memory", "semantic")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create semantic memory dir: %w", err)
	}

	s := &SemanticStore{
		facts:    make(map[string]SemanticFact),
		index:    NewInvertedIndex(),
		storeDir: storeDir,
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load semantic store: %w", err)
	}

	return s, nil
}

func (s *SemanticStore) Store(fact SemanticFact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fact.ID == "" {
		fact.ID = fmt.Sprintf("sf_%x", time.Now().UnixNano())
	}
	now := time.Now()
	if fact.CreatedAt.IsZero() {
		fact.CreatedAt = now
	}
	fact.UpdatedAt = now

	if fact.Confidence < 0 {
		fact.Confidence = 0
	}
	if fact.Confidence > 1 {
		fact.Confidence = 1
	}

	s.facts[fact.ID] = fact
	s.index.Index(fact.ID, append(fact.Tags, strings.Fields(strings.ToLower(fact.Value))...))

	return s.persist()
}

func (s *SemanticStore) RetrieveByCategory(category string) ([]SemanticFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SemanticFact
	for _, f := range s.facts {
		if f.Category == category {
			results = append(results, f)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Confidence > results[j].Confidence
	})
	return results, nil
}

func (s *SemanticStore) RetrieveByTags(tags []string) ([]SemanticFact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SemanticFact
	for _, f := range s.facts {
		if factHasTags(f, tags) {
			results = append(results, f)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Confidence > results[j].Confidence
	})
	return results, nil
}

func (s *SemanticStore) Get(id string) (SemanticFact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.facts[id]
	return f, ok
}

func (s *SemanticStore) UpdateConfidence(id string, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.facts[id]
	if !ok {
		return fmt.Errorf("fact %s not found", id)
	}

	f.Confidence += delta
	if f.Confidence < 0 {
		f.Confidence = 0
	}
	if f.Confidence > 1 {
		f.Confidence = 1
	}
	f.UpdatedAt = time.Now()
	s.facts[id] = f

	return s.persist()
}

func (s *SemanticStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.facts, id)
	s.index.Remove(id)
	return s.persist()
}

func (s *SemanticStore) List() []SemanticFact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SemanticFact, 0, len(s.facts))
	for _, f := range s.facts {
		results = append(results, f)
	}
	return results
}

func (s *SemanticStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.facts = make(map[string]SemanticFact)
	s.index = NewInvertedIndex()
	return s.persist()
}

func factHasTags(f SemanticFact, tags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range f.Tags {
		tagSet[t] = true
	}
	for _, t := range tags {
		if tagSet[t] {
			return true
		}
	}
	return false
}

func (s *SemanticStore) persist() error {
	data, err := json.MarshalIndent(s.facts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal semantic facts: %w", err)
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

func (s *SemanticStore) load() error {
	path := filepath.Join(s.storeDir, "store.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := json.Unmarshal(data, &s.facts); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	for _, f := range s.facts {
		s.index.Index(f.ID, append(f.Tags, strings.Fields(strings.ToLower(f.Value))...))
	}
	return nil
}

func FormatSemanticContext(facts []SemanticFact) string {
	if len(facts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<semantic_memory>\n")
	b.WriteString("Known facts and preferences:\n\n")

	for _, f := range facts {
		b.WriteString(fmt.Sprintf("[%s | confidence=%.1f]\n", f.Category, f.Confidence))
		b.WriteString(f.Key + ": " + f.Value + "\n\n")
	}

	b.WriteString("</semantic_memory>\n")
	return b.String()
}
