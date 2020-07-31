package qproxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// BackendStatistics stores backends statistics
type BackendStatistics struct {
	Name        string
	URL         string
	Sessions    int
	MaxSessions int
	SessionTTL  string
}

type backend struct {
	name         string
	url          *url.URL
	weight       float64
	sessionTTL   time.Duration
	maxSessions  int
	handler      *httputil.ReverseProxy
	sessionStore *sessionStore
}

func newBackend(name string, config *backendConfig, store *sessionStore) (*backend, error) {
	proxyURL, err := url.Parse(config.url)
	if err != nil {
		return nil, err
	}

	if store == nil {
		store = newSessionStore()
	}

	handler := httputil.NewSingleHostReverseProxy(proxyURL)
	handler.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.tlsInsecure},
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: config.maxSessions,
		IdleConnTimeout:     300 * time.Second,
	}

	return &backend{
		name:         name,
		url:          proxyURL,
		weight:       config.weight,
		sessionTTL:   config.sessionTTL,
		maxSessions:  config.maxSessions,
		handler:      handler,
		sessionStore: store,
	}, nil
}

func (b *backend) removeExpiredSessions() {
	b.sessionStore.removeExpired()
}

func (b *backend) remainingPlaces() int {
	remainingPlaces := b.maxSessions - b.sessionStore.len()
	if remainingPlaces < 0 {
		return 0
	}

	return remainingPlaces
}

func (b *backend) loadSession(id string) (*session, bool) {
	return b.sessionStore.load(id)
}

func (b *backend) storeSession(id string) (*session, bool) {
	if s, ok := b.sessionStore.load(id); ok {
		return s, true
	}

	if b.remainingPlaces() == 0 {
		return nil, false
	}

	return b.sessionStore.store(newSession(id, b.sessionTTL)), true
}

func (b *backend) statistics() *BackendStatistics {
	return &BackendStatistics{
		Name:        b.name,
		URL:         b.url.String(),
		SessionTTL:  b.sessionTTL.String(),
		Sessions:    b.sessionStore.len(),
		MaxSessions: b.maxSessions,
	}
}
