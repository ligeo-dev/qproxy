package qproxy

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// ListenAndServe starts the proxy and API HTTP server
func (qp *QProxy) ListenAndServe() {
	if !qp.isStarted.setTrue() {
		return
	}

	qp.startTime = time.Now()
	go qp.handleSessionUpdate()
	go qp.handleShutdownSignal()
	go qp.handleConfigurationReloadSignal()
	go qp.serveAPI()

	qp.serveProxy()
	<-qp.doneChan
}

// Shutdown stops the proxy and API HTTP server
func (qp *QProxy) Shutdown(ctx context.Context) {
	if !qp.isStarted.isTrue() || !qp.inShutdown.setTrue() {
		return
	}

	log.Info("QProxy server is shutting down....")
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if qp.server != nil {
			if err := qp.server.Shutdown(ctx); err != nil {
				log.WithFields(log.Fields{"error": err}).Warning("Could not gracefully shutdown QProxy server")
			}
		}

		wg.Done()
	}()
	go func() {
		if qp.apiServer != nil {
			if err := qp.apiServer.Shutdown(ctx); err != nil {
				log.WithFields(log.Fields{"error": err}).Warning("Could not gracefully shutdown Api server")
			}
		}

		wg.Done()
	}()

	wg.Wait()
	close(qp.doneChan)
}

func (qp *QProxy) handleShutdownSignal() {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, os.Kill)
	select {
	case <-sigint:
	case <-qp.doneChan:
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	qp.Shutdown(ctx)
	cancel()
}

func (qp *QProxy) handleConfigurationReloadSignal() {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, syscall.SIGUSR2)
	for {
		select {
		case <-sigint:
			qp.syncReloadConfiguration()
		case <-qp.doneChan:
			return
		}
	}
}

func (qp *QProxy) serveProxy() {
	addr := qp.config.getString("addr")
	certFile := qp.config.getString("tls.cert_file")
	keyFile := qp.config.getString("tls.key_file")
	qp.server = &http.Server{
		ReadHeaderTimeout: 2 * time.Second,
		Addr:              addr,
		Handler:           http.TimeoutHandler(newProxyHandler(qp), qp.config.getDuration("timeout"), "Service Unavailable"),
	}

	var err error
	if certFile == "" && keyFile == "" {
		log.WithFields(log.Fields{"protocol": "http", "addr": addr}).Info("QProxy started")
		err = qp.server.ListenAndServe()
	} else {
		log.WithFields(log.Fields{"protocol": "https", "addr": addr}).Info("QProxy started")
		err = qp.server.ListenAndServeTLS(certFile, keyFile)
	}

	if err != http.ErrServerClosed {
		log.WithFields(log.Fields{"error": err}).Fatal("Proxy server error")
	}
}

func (qp *QProxy) serveAPI() {
	addr := qp.config.getString("api.addr")
	certFile := qp.config.getString("api.tls.cert_file")
	keyFile := qp.config.getString("api.tls.key_file")
	qp.apiServer = &http.Server{
		Addr:    addr,
		Handler: newAPIHandler(qp),
	}

	var err error
	if certFile == "" && keyFile == "" {
		log.WithFields(log.Fields{"protocol": "http", "addr": addr}).Info("Api started")
		err = qp.apiServer.ListenAndServe()
	} else {
		log.WithFields(log.Fields{"protocol": "https", "addr": addr}).Info("Api started")
		err = qp.apiServer.ListenAndServeTLS(certFile, keyFile)
	}

	if err != http.ErrServerClosed {
		log.WithFields(log.Fields{"error": err}).Error("Api server error")
	}
}
