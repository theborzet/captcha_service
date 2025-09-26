package challenge

import (
	"sync"
	"time"
)

type ChallengeStore struct {
	mu       sync.RWMutex
	answers  map[string]challengeData
	expiries map[string]time.Time
}

type challengeData struct {
	answer     string
	x, y       int
	complexity int
}

func NewInMemoryStore() *ChallengeStore {
	store := &ChallengeStore{
		answers:  make(map[string]challengeData),
		expiries: make(map[string]time.Time),
	}
	go store.cleanup()
	return store
}

func (s *ChallengeStore) Set(challengeID, answer string, x, y, complexity int, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[challengeID] = challengeData{answer: answer, x: x, y: y, complexity: complexity}
	s.expiries[challengeID] = time.Now().Add(ttl)
}

func (s *ChallengeStore) Get(challengeID string) (string, int, int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expiry, exists := s.expiries[challengeID]
	if !exists || time.Now().After(expiry) {
		return "", 0, 0, false
	}
	data, exists := s.answers[challengeID]
	return data.answer, data.x, data.y, exists
}

func (s *ChallengeStore) GetComplexity(challengeID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, exists := s.answers[challengeID]
	if !exists {
		return 1
	}
	return data.complexity
}

func (s *ChallengeStore) Delete(challengeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.answers, challengeID)
	delete(s.expiries, challengeID)
}

func (s *ChallengeStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, expiry := range s.expiries {
			if time.Now().After(expiry) {
				delete(s.answers, id)
				delete(s.expiries, id)
			}
		}
		s.mu.Unlock()
	}
}
