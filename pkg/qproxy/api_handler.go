package qproxy

import (
	"encoding/json"
	"net/http"
)

type apiHandler struct {
	qp     *QProxy
	router http.Handler
}

func newAPIHandler(qp *QProxy) *apiHandler {
	router := http.NewServeMux()
	router.Handle("/statistics", newAPIStatisticsHandler(qp))
	router.HandleFunc("/template/full", func(rw http.ResponseWriter, r *http.Request) {
		qp.config.getTemplate("queue.full_template").Execute(rw, nil)
	})
	router.HandleFunc("/template/queue", func(rw http.ResponseWriter, r *http.Request) {
		qp.config.getTemplate("queue.template").Execute(rw, nil)
	})

	return &apiHandler{qp: qp, router: router}
}

func (handler *apiHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	apiUsername := handler.qp.config.getString("api.username")
	apiPassword := handler.qp.config.getString("api.password")
	if apiUsername == "" || apiPassword == "" {
		handler.router.ServeHTTP(rw, r)
		return
	}

	username, password, authOK := r.BasicAuth()
	if !authOK || username != apiUsername || password != apiPassword {
		rw.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(rw, "Unauthorized.", http.StatusUnauthorized)
		return
	}

	handler.router.ServeHTTP(rw, r)
}

type apiStatisticsHandler struct {
	qp *QProxy
}

func newAPIStatisticsHandler(qp *QProxy) *apiStatisticsHandler {
	return &apiStatisticsHandler{qp: qp}
}

func (handler *apiStatisticsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	statistics := handler.qp.syncStatistics()
	js, err := json.Marshal(statistics)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.Write(js)
}
