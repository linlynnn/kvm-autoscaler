package discovery

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
)

type targetConfig struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

type PromServiceDiscovery struct {
	targetConfigs map[string]targetConfig
}

func NewPromServiceDiscovery() *PromServiceDiscovery {

	targetConfigs := map[string]targetConfig{
		"node_exporter": targetConfig{
			Targets: []string{},
			Labels:  map[string]string{},
		},
	}

	return &PromServiceDiscovery{
		targetConfigs: targetConfigs,
	}
}

func (s *PromServiceDiscovery) Run() {
	r := chi.NewRouter()

	r.Get("/targets/node_exporter", s.getNodeExporterTargetHandler)
	r.Delete("/targets/node_exporter", s.deleteNodeExporterTargetHandler)
	r.Post("/targets/node_exporter", s.createCpuNodeExporterTargetHandler)

	log.Printf("Service Discovery running on %s\n", "9093")
	log.Println(http.ListenAndServe(":9093", r))

}

func (s *PromServiceDiscovery) getNodeExporterService() []targetConfig {
	return []targetConfig{
		s.targetConfigs["node_exporter"],
	}
}

func (s *PromServiceDiscovery) getNodeExporterTargetHandler(w http.ResponseWriter, r *http.Request) {
	result := s.getNodeExporterService()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)

}

func (s *PromServiceDiscovery) deleteNodeExporterTargetService(url string) {
	nodeExporterTarget := s.targetConfigs["node_exporter"]

	for idx, target := range nodeExporterTarget.Targets {
		if target == url {
			nodeExporterTarget.Targets = slices.Delete(nodeExporterTarget.Targets, idx, idx+1)
			break // Assuming only one occurrence
		}
	}

	s.targetConfigs["node_exporter"] = nodeExporterTarget
}

func (s *PromServiceDiscovery) deleteNodeExporterTargetHandler(w http.ResponseWriter, r *http.Request) {

	var deleteRequest deleteNodeExporterTargetRequest

	err := json.NewDecoder(r.Body).Decode(&deleteRequest)
	if err != nil {
		http.Error(w, "invalid body request", http.StatusBadRequest)
		return
	}

	s.deleteNodeExporterTargetService(deleteRequest.Url)
	w.WriteHeader(http.StatusAccepted)

}

func (s *PromServiceDiscovery) createCpuNodeExporterTargetService(url string) {
	nodeExporterTarget := s.targetConfigs["node_exporter"]

	for _, target := range nodeExporterTarget.Targets {
		if target == url {
			return
		}
	}

	nodeExporterTarget.Targets = append(nodeExporterTarget.Targets, url)
	s.targetConfigs["node_exporter"] = nodeExporterTarget
}

func (s *PromServiceDiscovery) createCpuNodeExporterTargetHandler(w http.ResponseWriter, r *http.Request) {
	var createRequest createNodeExporterTargetRequest

	err := json.NewDecoder(r.Body).Decode(&createRequest)
	if err != nil {
		http.Error(w, "invalid body request", http.StatusBadRequest)
	}

	s.createCpuNodeExporterTargetService(createRequest.Url)
	w.WriteHeader(http.StatusCreated)

}
