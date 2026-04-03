package memory

import (
	"math"
	"sort"
	"strings"
	"sync"
)

type InvertedIndex struct {
	mu       sync.RWMutex
	postings map[string]map[string]int
	docFreq  map[string]int
}

func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		postings: make(map[string]map[string]int),
		docFreq:  make(map[string]int),
	}
}

func (idx *InvertedIndex) Index(docID string, terms []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	seen := make(map[string]bool)
	for _, term := range terms {
		term = strings.ToLower(term)
		if len(term) < 2 || seen[term] {
			continue
		}
		seen[term] = true

		if idx.postings[term] == nil {
			idx.postings[term] = make(map[string]int)
		}
		idx.postings[term][docID]++
	}

	for term := range seen {
		idx.docFreq[term]++
	}
}

func (idx *InvertedIndex) Remove(docID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for term, docs := range idx.postings {
		if _, ok := docs[docID]; ok {
			delete(docs, docID)
			idx.docFreq[term]--
			if idx.docFreq[term] <= 0 {
				delete(idx.postings, term)
				delete(idx.docFreq, term)
			}
		}
	}
}

type ScoredResult struct {
	DocID string
	Score float64
}

func (idx *InvertedIndex) Search(query []string, topK int) []ScoredResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	scores := make(map[string]float64)
	N := float64(len(idx.postings)) + 1

	for _, term := range query {
		term = strings.ToLower(term)
		df := float64(idx.docFreq[term])
		if df == 0 {
			continue
		}
		idf := math.Log(N / df)

		for docID, freq := range idx.postings[term] {
			scores[docID] += idf * float64(freq)
		}
	}

	results := make([]ScoredResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, ScoredResult{DocID: docID, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results
}

func (idx *InvertedIndex) RelevanceScore(query []string, docTerms []string) float64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(query) == 0 || len(docTerms) == 0 {
		return 0
	}

	querySet := make(map[string]bool)
	for _, t := range query {
		querySet[strings.ToLower(t)] = true
	}

	var score float64
	for _, t := range docTerms {
		if querySet[strings.ToLower(t)] {
			score++
		}
	}

	return score / float64(len(query))
}
