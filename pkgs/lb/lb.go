package lb

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type LoadBalancer struct {
	backends []*Backend
	current  uint64
	address  string
	mu       sync.Mutex
}

type LoadCpuUtilRequest struct {
	Cores   int `json:"cores"`
	Util    int `json:"util"`
	Timeout int `json:"timeout"`
}

func NewLoadBalancer(address string) *LoadBalancer {
	return &LoadBalancer{
		backends: []*Backend{},
		address:  address,
	}
}

func (lb *LoadBalancer) loadCpuUtilBackend(BackendURL string, Cores int, Util int, Timeout int) {

	payload := map[string]string{
		"cores":   strconv.Itoa(Cores),
		"util":    strconv.Itoa(Util),
		"timeout": strconv.Itoa(Timeout),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(BackendURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Println(resp.StatusCode)

}

func (lb *LoadBalancer) loadCpuUtilAll(loadCpuUtilRequest LoadCpuUtilRequest) {
	var alivedBackends []*Backend

	lb.mu.Lock()
	for _, backend := range lb.backends {
		if backend.State == BACKEND_STATE_ALIVE {
			alivedBackends = append(alivedBackends, backend)
		}
	}
	lb.mu.Unlock()

	for _, backend := range alivedBackends {
		// fire CPU Util
		go lb.loadCpuUtilBackend(
			backend.URL.String()+"/load/cpu",
			loadCpuUtilRequest.Cores,
			loadCpuUtilRequest.Util,
			loadCpuUtilRequest.Timeout,
		)

	}

}

func (lb *LoadBalancer) LoadCpuUtilHandler(w http.ResponseWriter, r *http.Request) {

	var loadCpuUtilizeRequest LoadCpuUtilRequest
	err := json.NewDecoder(r.Body).Decode(&loadCpuUtilizeRequest)
	if err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	lb.loadCpuUtilAll(loadCpuUtilizeRequest)

}

func (lb *LoadBalancer) GetAddress() string {
	return lb.address
}

func (lb *LoadBalancer) Run() {

	r := chi.NewRouter()

	r.Get("/", lb.ServeHTTP)
	r.Get("/backend", lb.GetBackendListHandler)
	r.Post("/backend", lb.RegisterBackendHandler)
	r.Delete("/backend", lb.DeRegisterHandler)
	r.Post("/load/cpu", lb.LoadCpuUtilHandler)

	log.Printf("Load balancer running on %s\n", lb.address)
	log.Println(http.ListenAndServe(lb.address, r))

}

func (lb *LoadBalancer) getNextBackend() *Backend {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.backends) == 0 {
		log.Println("no any backend")
		return nil
	}

	start := lb.current
	for {
		b := lb.backends[lb.current%uint64(len(lb.backends))]
		lb.current++

		if b.IsAlive() && !b.IsDraining() {
			return b
		}

		if lb.current%uint64(len(lb.backends)) == start {
			return nil // No healthy backend
		}
	}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.getNextBackend()
	if backend == nil {
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		return
	}
	backend.Proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) registerBackend(ipAddress string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	log.Printf("Registering backend %s\n", ipAddress)
	newBackend := NewBackend(ipAddress)
	// start healthcheck the backend
	lb.backends = append(lb.backends, newBackend)
	log.Printf("Registered backend %s\n", ipAddress)

	go lb.startHealthCheck(newBackend, 5*time.Second)

}

func (lb *LoadBalancer) RegisterBackendHandler(w http.ResponseWriter, r *http.Request) {
	var registerBackendRequest RegisterBackendRequest

	err := json.NewDecoder(r.Body).Decode(&registerBackendRequest)
	if err != nil {
		http.Error(w, "invalid body request", http.StatusBadRequest)
		return
	}

	lb.registerBackend(registerBackendRequest.URL)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Successfully registered backend"))

}

func (lb *LoadBalancer) startHealthCheck(backend *Backend, interval time.Duration) {

	log.Printf("Waiting for startup backend %s for 1 minute\n", backend.URL.String())
	time.Sleep(1 * time.Minute)
	log.Printf("Start health check %s\n", backend.URL.String())

	for {
		resp, err := http.Get(backend.URL.String() + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Printf("%s Not Alive", backend.URL.String())
			backend.SetStateAlive(false)
			lb.deRegister(backend.URL.String())
			break
		} else {
			backend.SetStateAlive(true)
		}
		time.Sleep(interval)
	}

}

func (lb *LoadBalancer) getBackendList(status string) []BackendResponse {
	var result []BackendResponse

	for _, b := range lb.backends {
		b.mu.RLock()
		isAlive := b.IsAlive()
		isDraining := b.IsDraining()
		url := b.URL
		b.mu.RUnlock()

		if status == "alive" && !isAlive {
			continue
		}

		if status == "draining" && !isDraining {
			continue
		}

		result = append(result, BackendResponse{
			URL: url.String(),
		})
	}

	return result

}

func (lb *LoadBalancer) GetBackendListHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	statusVal := query.Get("status")

	result := lb.getBackendList(statusVal)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (lb *LoadBalancer) deRegister(url string) {
	lb.mu.Lock()

	idxToRemove := -1
	var backendToRemove *Backend

	for idx, backend := range lb.backends {
		if backend.URL.String() == url {
			idxToRemove = idx
			backendToRemove = backend
			break
		}
	}

	if idxToRemove == -1 {
		lb.mu.Unlock()
		log.Printf("deRegister: no backend %s\n", url)
		return
	}

	backendToRemove.SetStateDraining(true)
	log.Printf("Draining %s\n", url)

	lb.mu.Unlock()

	go func() {
		time.Sleep(30 * time.Second)
		lb.mu.Lock()
		for idxToRemove, backend := range lb.backends {
			if backend.URL.String() == url {
				lb.backends = slices.Delete(lb.backends, idxToRemove, idxToRemove+1)
				break
			}
		}
		log.Printf("Deregistered %s\n", url)
		lb.mu.Unlock()

	}()

}

func (lb *LoadBalancer) DeRegisterHandler(w http.ResponseWriter, r *http.Request) {

	var deRegisterBackendRequest DeRegisterBackendRequest

	err := json.NewDecoder(r.Body).Decode(&deRegisterBackendRequest)
	if err != nil {
		http.Error(w, "invalid body request", http.StatusBadRequest)
		return
	}

	lb.deRegister(deRegisterBackendRequest.URL)
	w.WriteHeader(http.StatusAccepted)

}
