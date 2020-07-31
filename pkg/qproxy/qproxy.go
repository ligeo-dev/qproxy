package qproxy

import (
	"errors"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type atomicBool int32

func (b *atomicBool) isTrue() bool  { return atomic.LoadInt32((*int32)(b)) == 1 }
func (b *atomicBool) setTrue() bool { return atomic.CompareAndSwapInt32((*int32)(b), 0, 1) }

type ipList struct {
	ipList    []net.IP
	ipNetList []*net.IPNet
}

func (l *ipList) contains(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, ipNet := range l.ipNetList {
		if ipNet.Contains(parsedIP) {
			return true
		}
	}

	for _, i := range l.ipList {
		if i.Equal(parsedIP) {
			return true
		}
	}

	return false
}

func newIPList(ips []string) (*ipList, error) {
	ipSlice := make([]net.IP, 0)
	ipNetSlice := make([]*net.IPNet, 0)
	for _, ip := range ips {
		if strings.Contains(ip, "/") {
			_, ipNet, err := net.ParseCIDR(ip)
			if err != nil {
				return nil, err
			}

			ipNetSlice = append(ipNetSlice, ipNet)
			continue
		}

		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return nil, errors.New("Invalid IP: " + ip)
		}

		ipSlice = append(ipSlice, parsedIP)
	}

	return &ipList{ipList: ipSlice, ipNetList: ipNetSlice}, nil
}

// ProxyStatistics stores proxy statistics
type ProxyStatistics struct {
	Uptime            string
	QueuedSessions    int
	MaxQueuedSessions int
	QueuedSessionTTL  string
	Backends          []*BackendStatistics
}

// QProxy stores sessions and disptach them to backends
type QProxy struct {
	config         *proxyConfig
	isStarted      atomicBool
	inShutdown     atomicBool
	doneChan       chan struct{}
	startTime      time.Time
	server         *http.Server
	apiServer      *http.Server
	atomicBackends atomic.Value
	sessionsLock   sync.RWMutex
	queuedSessions *sessionStore
}

// NewQProxy create a Proxy using Viper
func NewQProxy(v *viper.Viper) (*QProxy, error) {
	config, err := newQProxyConfig(v)
	if err != nil {
		return nil, err
	}

	qp := QProxy{
		config:         config,
		doneChan:       make(chan struct{}),
		queuedSessions: newSessionStore(),
	}

	backends := make([]*backend, 0)
	for backendName, backendConfig := range config.getBackendsConfig() {
		backend, err := newBackend(backendName, backendConfig, nil)
		if err != nil {
			return nil, err
		}

		backends = append(backends, backend)
	}
	qp.atomicBackends.Store(backends)

	return &qp, nil
}

// Start is an helper method to start QProxy
func Start() {
	qp, err := NewQProxy(viper.GetViper())
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Unable to start QProxy")
	}

	qp.ListenAndServe()
}

func (qp *QProxy) handleSessionUpdate() {
	ticker := time.NewTicker(qp.config.getDuration("session_refresh_interval"))
	reloadNotifyChan := qp.config.reloadNotifyChan()
	for {
		select {
		case <-qp.doneChan:
			ticker.Stop()
			return
		case <-ticker.C:
			qp.syncUpdateSessions()
		case <-reloadNotifyChan:
			ticker.Stop()
			ticker = time.NewTicker(qp.config.getDuration("session_refresh_interval"))
		}
	}
}

func (qp *QProxy) syncReloadConfiguration() {
	if err := qp.config.syncReload(); err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Unable to reload configuration")
		return
	}

	oldBackends := qp.backends()
	newBackends := make([]*backend, 0)
	for backendName, backendConfig := range qp.config.getBackendsConfig() {
		var sessionStore *sessionStore
		for _, oldBackend := range oldBackends {
			if oldBackend.name == backendName {
				sessionStore = oldBackend.sessionStore
				break
			}
		}

		backend, err := newBackend(backendName, backendConfig, sessionStore)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Unable to reload configuration")
			return
		}

		newBackends = append(newBackends, backend)
	}
	qp.atomicBackends.Store(newBackends)
	log.Info("Configuration reloaded")
}

func (qp *QProxy) randomBackend() *backend {
	backends := qp.backends()

	return backends[rand.Intn(len(backends))]
}

func (qp *QProxy) backends() []*backend {
	return qp.atomicBackends.Load().([]*backend)
}

func (qp *QProxy) availableBackends() []*backend {
	availableBackends := make([]*backend, 0)
	for _, backend := range qp.backends() {
		if backend.remainingPlaces() > 0 {
			availableBackends = append(availableBackends, backend)
		}
	}

	return availableBackends
}

func (qp *QProxy) isValidSessionID(id string) bool {
	if id == "" {
		return false
	}

	_, err := xid.FromString(id)

	return err == nil
}

func (qp *QProxy) syncHasRemainingQueueSlots() bool {
	qp.sessionsLock.RLock()
	hasRemainingQueueSlots := qp.hasRemainingQueueSlots()
	qp.sessionsLock.RUnlock()

	return hasRemainingQueueSlots
}

func (qp *QProxy) hasRemainingQueueSlots() bool {
	maxQueuedSessions := qp.config.getInt("queue.max_sessions")
	if maxQueuedSessions <= 0 {
		return true
	}

	return (maxQueuedSessions - qp.queuedSessions.len()) > 0
}

func (qp *QProxy) loadSession(id string) (*session, *backend, bool) {
	for _, backend := range qp.backends() {
		if session, ok := backend.loadSession(id); ok {
			session.update(backend.sessionTTL)

			return session, backend, true
		}
	}

	if session, ok := qp.queuedSessions.load(id); ok {
		session.update(qp.config.getDuration("queue.session_ttl"))

		return session, nil, true
	}

	return nil, nil, false
}

func (qp *QProxy) syncLoadSession(id string) (*session, *backend, bool) {
	qp.sessionsLock.RLock()
	session, backend, ok := qp.loadSession(id)
	qp.sessionsLock.RUnlock()

	return session, backend, ok
}

func (qp *QProxy) syncNewSession() (*session, *backend, bool) {
	qp.sessionsLock.Lock()
	defer qp.sessionsLock.Unlock()

	id := xid.New().String()

	if qp.queuedSessions.len() == 0 {
		backends := qp.availableBackends()
		if len(backends) > 0 {
			rand.Shuffle(len(backends), func(i int, j int) {
				backends[i], backends[j] = backends[j], backends[i]
			})
			rndWeight := rand.Float64()
			backendsLastIdx := len(backends) - 1
			for idx, backend := range backends {
				if backend.weight >= rndWeight || idx == backendsLastIdx {
					if session, ok := backend.storeSession(id); ok {
						return session, backend, true
					}
				}
			}
		}
	}

	if !qp.hasRemainingQueueSlots() {
		return nil, nil, false
	}

	return qp.queuedSessions.store(newSession(id, qp.config.getDuration("queue.session_ttl"))), nil, true
}

func (qp *QProxy) syncUpdateSessions() {
	qp.sessionsLock.Lock()
	defer qp.sessionsLock.Unlock()

	freeSlots := 0
	availableBackends := make([]*backend, 0)
	qp.queuedSessions.removeExpired()
	for _, backend := range qp.backends() {
		backend.removeExpiredSessions()
		if remainingPlaces := backend.remainingPlaces(); remainingPlaces > 0 {
			freeSlots += remainingPlaces
			availableBackends = append(availableBackends, backend)
		}
	}

	if freeSlots == 0 || qp.queuedSessions.len() == 0 {
		return
	}

	for _, session := range qp.queuedSessions.pop(freeSlots) {
		// On mélange la liste des backends pour éviter de toujours ajouter les sessions sur le même
		rand.Shuffle(len(availableBackends), func(i int, j int) {
			availableBackends[i], availableBackends[j] = availableBackends[j], availableBackends[i]
		})
		// On génère une valeur de poids aléatoire
		rndWeight := rand.Float64()
		var stored bool
		// Dans cette boucle on on tente d'affecter la session à un backend avec de la place
		// disponnible tout en prennant en compte la notion de poids
		for _, backend := range availableBackends {
			if backend.remainingPlaces() == 0 {
				continue
			}
			if rndWeight > backend.weight {
				continue
			}
			if _, ok := backend.storeSession(session.id); ok {
				stored = true
				break
			}
		}
		// Si la session a été affectée on passe à la suivante
		if stored {
			continue
		}
		// Dans cette boucle on tente d'affecter la session au premier backend avec de la place
		for _, backend := range availableBackends {
			if backend.remainingPlaces() == 0 {
				continue
			}
			if _, ok := backend.storeSession(session.id); ok {
				stored = true
				break
			}
		}
		// Si la session a été affectée on passe à la suivante
		if stored {
			continue
		}
		// Sinon on replace la session au début de la file d'attente
		qp.queuedSessions.unshift(session)
	}
}

func (qp *QProxy) syncStatistics() *ProxyStatistics {
	qp.sessionsLock.RLock()
	statistics := ProxyStatistics{
		Uptime:            time.Now().Sub(qp.startTime).String(),
		QueuedSessions:    qp.queuedSessions.len(),
		MaxQueuedSessions: qp.config.getInt("queue.max_sessions"),
		QueuedSessionTTL:  qp.config.getDuration("queue.session_ttl").String(),
		Backends:          make([]*BackendStatistics, 0),
	}

	for _, backend := range qp.backends() {
		statistics.Backends = append(statistics.Backends, backend.statistics())
	}
	qp.sessionsLock.RUnlock()

	return &statistics
}

func (qp *QProxy) getClientIP(r *http.Request) (string, bool) {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Error while processing request")
		return "", false
	}

	trustedProxies := qp.config.getIPList("trusted_proxies")
	if !trustedProxies.contains(remoteIP) {
		return remoteIP, true
	}

	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ", ")
		for i := len(ips) - 1; i >= 0; i-- {
			ip := ips[i]
			if !trustedProxies.contains(ip) {
				return ip, true
			}
		}
	}

	log.WithFields(log.Fields{
		"error":           "Unable to guess client IP",
		"remote-addr":     remoteIP,
		"x-forwarded-for": forwardedFor,
	}).Error("Error while processing request")
	return "", false
}

func (qp *QProxy) isRequestWhitelisted(r *http.Request) bool {
	if clientIP, ok := qp.getClientIP(r); ok {
		return qp.config.getIPList("whitelisted_ips").contains(clientIP)
	}

	return false
}
