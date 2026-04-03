package memory

import (
	"sync"
	"time"
)

type WorkingMemory struct {
	mu        sync.RWMutex
	facts     map[string]Fact
	context   map[string]string
	sessionID string
	maxSize   int
}

type Fact struct {
	Key        string
	Value      string
	Source     string
	Confidence float64
	CreatedAt  time.Time
}

func NewWorkingMemory(sessionID string, maxSize int) *WorkingMemory {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &WorkingMemory{
		facts:     make(map[string]Fact),
		context:   make(map[string]string),
		sessionID: sessionID,
		maxSize:   maxSize,
	}
}

func (wm *WorkingMemory) Set(key, value string, source string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.facts) >= wm.maxSize {
		wm.evictOldestLocked()
	}

	wm.facts[key] = Fact{
		Key:        key,
		Value:      value,
		Source:     source,
		Confidence: 0.8,
		CreatedAt:  time.Now(),
	}
}

func (wm *WorkingMemory) SetWithConfidence(key, value string, source string, confidence float64) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.facts) >= wm.maxSize {
		wm.evictOldestLocked()
	}

	wm.facts[key] = Fact{
		Key:        key,
		Value:      value,
		Source:     source,
		Confidence: confidence,
		CreatedAt:  time.Now(),
	}
}

func (wm *WorkingMemory) Get(key string) (Fact, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	f, ok := wm.facts[key]
	return f, ok
}

func (wm *WorkingMemory) SetContext(key, value string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.context[key] = value
}

func (wm *WorkingMemory) GetContext(key string) (string, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	v, ok := wm.context[key]
	return v, ok
}

func (wm *WorkingMemory) List() []Fact {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	result := make([]Fact, 0, len(wm.facts))
	for _, f := range wm.facts {
		result = append(result, f)
	}
	return result
}

func (wm *WorkingMemory) SessionID() string {
	return wm.sessionID
}

func (wm *WorkingMemory) Clear() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.facts = make(map[string]Fact)
	wm.context = make(map[string]string)
}

func (wm *WorkingMemory) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	found := false

	for key, f := range wm.facts {
		if !found || f.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = f.CreatedAt
			found = true
		}
	}

	if found {
		delete(wm.facts, oldestKey)
	}
}
