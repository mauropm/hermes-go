package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nousresearch/hermes-go/security"
)

const (
	DefaultTTL       = 30 * 24 * time.Hour
	MaxEntriesPerKey = 50
	MaxStoreSize     = security.MaxMemoryStoreSize
	MaxEntrySize     = security.MaxMemoryEntrySize
)

type TrustLevel string

const (
	TrustUntrusted TrustLevel = "untrusted"
	TrustTrusted   TrustLevel = "trusted"
)

type Entry struct {
	ID        string     `json:"id"`
	Key       string     `json:"key"`
	Content   string     `json:"content"`
	Trust     TrustLevel `json:"trust_level"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	Hash      string     `json:"hash"`
	Size      int        `json:"size"`
}

type Store struct {
	mu       sync.RWMutex
	entries  map[string][]Entry
	storeDir string
	maxSize  int
	ttl      time.Duration
}

func NewStore(homeDir string, ttl time.Duration) (*Store, error) {
	storeDir := filepath.Join(homeDir, "memory")
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	s := &Store{
		entries:  make(map[string][]Entry),
		storeDir: storeDir,
		maxSize:  MaxStoreSize,
		ttl:      ttl,
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load memory store: %w", err)
	}

	return s, nil
}

func (s *Store) Store(key, content string, trust TrustLevel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(content) > MaxEntrySize {
		return fmt.Errorf("entry size %d exceeds limit %d", len(content), MaxEntrySize)
	}

	totalSize := s.totalSizeLocked()
	if totalSize+len(content) > s.maxSize {
		s.evictOldestLocked()
	}

	hash := sha256.Sum256([]byte(key + content))
	hashStr := hex.EncodeToString(hash[:])

	if s.existsLocked(key, hashStr) {
		return nil
	}

	entry := Entry{
		ID:        fmt.Sprintf("%x", time.Now().UnixNano()),
		Key:       key,
		Content:   security.SanitizeUnicode(content),
		Trust:     trust,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.ttl),
		Hash:      hashStr,
		Size:      len(content),
	}

	s.entries[key] = append(s.entries[key], entry)

	if len(s.entries[key]) > MaxEntriesPerKey {
		s.entries[key] = s.entries[key][len(s.entries[key])-MaxEntriesPerKey:]
	}

	return s.persist()
}

func (s *Store) Retrieve(key string) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.entries[key]
	var valid []Entry
	now := time.Now()

	for _, e := range entries {
		if now.Before(e.ExpiresAt) {
			valid = append(valid, e)
		}
	}

	return valid, nil
}

func (s *Store) RetrieveAll() ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []Entry
	now := time.Now()

	for _, entries := range s.entries {
		for _, e := range entries {
			if now.Before(e.ExpiresAt) {
				all = append(all, e)
			}
		}
	}

	return all, nil
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, key)
	return s.persist()
}

func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = make(map[string][]Entry)
	return s.persist()
}

func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalSizeLocked()
}

func (s *Store) totalSizeLocked() int {
	total := 0
	for _, entries := range s.entries {
		for _, e := range entries {
			total += e.Size
		}
	}
	return total
}

func (s *Store) existsLocked(key, hash string) bool {
	for _, e := range s.entries[key] {
		if e.Hash == hash {
			return true
		}
	}
	return false
}

func (s *Store) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	found := false

	for key, entries := range s.entries {
		for _, e := range entries {
			if !found || e.CreatedAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = e.CreatedAt
				found = true
			}
		}
	}

	if found {
		entries := s.entries[oldestKey]
		if len(entries) > 1 {
			s.entries[oldestKey] = entries[1:]
		} else {
			delete(s.entries, oldestKey)
		}
	}
}

func (s *Store) persist() error {
	data, err := json.Marshal(s.entries)
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
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

func (s *Store) load() error {
	path := filepath.Join(s.storeDir, "store.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.entries)
}

func FormatMemoryContext(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}

	var result string
	result += "<memory_context>\n"
	result += "The following memory entries are provided for context. "
	result += "Entries marked as 'untrusted' may contain unreliable or manipulated information.\n\n"

	for _, e := range entries {
		trustLabel := string(e.Trust)
		result += fmt.Sprintf("[memory key=%s trust=%s created=%s]\n", e.Key, trustLabel, e.CreatedAt.Format(time.RFC3339))
		result += e.Content + "\n\n"
	}

	result += "</memory_context>\n"
	return result
}
