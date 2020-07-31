package qproxy

import (
	"sync/atomic"
	"time"
)

type session struct {
	id               string
	atomicExpiration atomic.Value
}

func newSession(id string, ttl time.Duration) *session {
	s := session{id: id}
	s.update(ttl)

	return &s
}

func (s *session) update(ttl time.Duration) {
	s.atomicExpiration.Store(time.Now().Add(ttl))
}

func (s *session) expiration() time.Time {
	return s.atomicExpiration.Load().(time.Time)
}

type sessionStore struct {
	sessions []*session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make([]*session, 0)}
}

func (store *sessionStore) load(id string) (*session, bool) {
	for _, s := range store.sessions {
		if s.id == id {
			return s, true
		}
	}

	return nil, false
}

func (store *sessionStore) store(session *session) *session {
	if s, ok := store.load(session.id); ok {
		return s
	}

	store.sessions = append(store.sessions, session)

	return session
}

func (store *sessionStore) removeExpired() {
	if len(store.sessions) == 0 {
		return
	}

	sessions := store.sessions[:0]
	now := time.Now()
	for _, session := range store.sessions {
		if session.expiration().After(now) {
			sessions = append(sessions, session)
		}
	}

	for i := len(sessions); i < len(store.sessions); i++ {
		store.sessions[i] = nil
	}

	store.sessions = sessions
}

func (store *sessionStore) pop(size int) []*session {
	if size > len(store.sessions) {
		size = len(store.sessions)
	}

	var sessions []*session
	sessions, store.sessions = store.sessions[:size], store.sessions[size:]

	return sessions
}

func (store *sessionStore) unshift(s *session) bool {
	if _, ok := store.load(s.id); ok {
		return false
	}
	store.sessions = append([]*session{s}, store.sessions...)
	return true
}

func (store *sessionStore) len() int {
	return len(store.sessions)
}
