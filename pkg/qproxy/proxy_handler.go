package qproxy

import "net/http"

type proxyHandler struct {
	qp *QProxy
}

func newProxyHandler(qp *QProxy) *proxyHandler {
	return &proxyHandler{qp: qp}
}

func (handler *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	qp := handler.qp
	if qp.isRequestWhitelisted(r) {
		qp.randomBackend().handler.ServeHTTP(rw, r)
		return
	}

	var sessionID string
	cookieName := qp.config.getString("cookie_name")
	if sessionCookie, err := r.Cookie(cookieName); err == nil {
		sessionID = sessionCookie.Value
	}

	var session *session
	var backend *backend
	if qp.isValidSessionID(sessionID) {
		session, backend, _ = qp.syncLoadSession(sessionID)
	} else if !qp.syncHasRemainingQueueSlots() {
		qp.config.getTemplate("queue.full_template").Execute(rw, nil)
		return
	}

	if session == nil {
		var ok bool
		session, backend, ok = qp.syncNewSession()
		if !ok {
			qp.config.getTemplate("queue.full_template").Execute(rw, nil)
			return
		}

		http.SetCookie(rw, &http.Cookie{
			Name:     cookieName,
			Path:     "/",
			Value:    session.id,
			HttpOnly: true,
		})
	}

	if backend != nil {
		backend.handler.ServeHTTP(rw, r)
		return
	}

	qp.config.getTemplate("queue.template").Execute(rw, nil)
}
