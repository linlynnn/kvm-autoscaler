package discovery

type createNodeExporterTargetRequest struct {
	Url string `json:"url"`
}

type deleteNodeExporterTargetRequest struct {
	Url string `json:"url"`
}
