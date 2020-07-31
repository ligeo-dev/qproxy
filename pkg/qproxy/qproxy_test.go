package qproxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/prometheus/common/log"
	"github.com/stretchr/testify/assert"
)

const testBackendAddr = "127.0.0.1:6464"
const testAddr = "127.0.0.1:6363"
const apiAddr = "127.0.0.1:6364"

var testBackendServer *http.Server

func TestNewQProxy(t *testing.T) {
	v := newViper()
	v.Set("backends.test.url", "http://"+testBackendAddr)
	v.Set("backends.test.max_sessions", 1)
	v.Set("backends.test.session_ttl", 5)

	_, err := NewQProxy(v)
	assert.NoError(t, err)
}

func createDummy() *QProxy {
	v := newViper()
	v.Set("backends.test.url", "http://"+testBackendAddr)
	v.Set("backends.test.max_sessions", 1)
	v.Set("backends.test.session_ttl", 5)

	qp, _ := NewQProxy(v)

	return qp
}

func listenAndServeTestBackend() {
	testBackendServer = &http.Server{
		Addr: testBackendAddr,
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Write([]byte("ok"))
		}),
	}

	if err := testBackendServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalln(err)
	}
}

func shutdownTestBackend() {
	if testBackendServer == nil {
		return
	}

	testBackendServer.Shutdown(context.Background())
}
