package lb

import (
	"log"
	"net/http/httputil"
	"net/url"
	"sync"
)

type BackendState int

const (
	BACKEND_STATE_ALIVE BackendState = iota
	BACKEND_STATE_DRAINING
)

type Backend struct {
	URL   *url.URL
	State BackendState
	mu    sync.RWMutex
	Proxy *httputil.ReverseProxy
}

type RegisterBackendRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type BackendResponse struct {
	URL string `json:"url"`
}

type BackendListRequest struct {
	Status string `json:"status"`
}

type DeRegisterBackendRequest struct {
	URL string `json:"url"`
}

func (b *Backend) SetStateAlive(alive bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.State = BACKEND_STATE_ALIVE
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.State == BACKEND_STATE_ALIVE {
		return true
	}

	return false
}

func (b *Backend) IsDraining() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.State == BACKEND_STATE_DRAINING {
		return true
	}
	return false
}

func (b *Backend) SetStateDraining(draining bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.State = BACKEND_STATE_DRAINING
}

func NewBackend(ipAddress string) *Backend {
	parsedIpAddress, err := url.Parse(ipAddress)
	if err != nil {
		log.Println("Parse IpAddress failed")
	}

	proxy := httputil.NewSingleHostReverseProxy(parsedIpAddress)
	backend := &Backend{
		URL:   parsedIpAddress,
		State: BACKEND_STATE_ALIVE,
		Proxy: proxy,
	}

	return backend

}
