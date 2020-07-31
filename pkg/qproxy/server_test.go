package qproxy

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenAndServe(t *testing.T) {
	qp := createDummy()
	go listenAndServeTestBackend()
	go qp.ListenAndServe()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var resp *http.Response
	for resp == nil {
		resp, _ = client.Get("http://" + testAddr)
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ok")
	var sessCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "qpid" {
			sessCookie = cookie
		}
	}

	require.NotNil(t, sessCookie)

	var respAPI *http.Response
	for respAPI == nil {
		respAPI, _ = client.Get("http://" + apiAddr + "/statistics")
	}

	defer respAPI.Body.Close()
	assert.Equal(t, 200, respAPI.StatusCode)
	bodyAPI, _ := ioutil.ReadAll(respAPI.Body)
	assert.Contains(t, string(bodyAPI), "Uptime")

	qp.Shutdown(context.Background())
	shutdownTestBackend()
}
