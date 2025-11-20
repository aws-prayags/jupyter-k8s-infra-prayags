package extensionapi

import (
	"encoding/json"
	"net/http"
	"strings"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
)

// v1 format structures (current)
type APIResourceListV1 struct {
	Kind         string        `json:"kind"`
	APIVersion   string        `json:"apiVersion"`
	GroupVersion string        `json:"groupVersion"`
	Resources    []APIResource `json:"resources"`
}

type APIResource struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Verbs        []string `json:"verbs"`
}

// v2 format structures
type APIResourceListV2 struct {
	Kind       string                 `json:"kind"`
	APIVersion string                 `json:"apiVersion"`
	Metadata   map[string]interface{} `json:"metadata"`
	Resources  []APIResourceV2        `json:"resources"`
}

type APIResourceV2 struct {
	Resource         string       `json:"resource"`
	ResponseKind     ResponseKind `json:"responseKind"`
	Scope            string       `json:"scope"`
	SingularResource string       `json:"singularResource"`
	Verbs            []string     `json:"verbs"`
}

type ResponseKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// handleDiscovery responds with API resource discovery information
// Supports both v1 and v2 aggregated discovery formats
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	acceptHeader := r.Header.Get("Accept")
	setupLog.Info("Discovery request received", "accept", acceptHeader, "path", r.URL.Path)

	// Check if client wants v2 format
	if strings.Contains(acceptHeader, "apidiscovery.k8s.io/v2") {
		setupLog.Info("Serving v2 discovery format")
		s.handleDiscoveryV2(w, r)
		return
	}

	// Default to v1 format
	setupLog.Info("Serving v1 discovery format")
	s.handleDiscoveryV1(w, r)
}

// handleDiscoveryV1 returns discovery in v1 format (original behavior)
func (s *ExtensionServer) handleDiscoveryV1(w http.ResponseWriter, r *http.Request) {
	response := APIResourceListV1{
		Kind:         "APIResourceList",
		APIVersion:   "v1",
		GroupVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
		Resources: []APIResource{
			{
				Name:         "workspaceconnections",
				SingularName: "workspaceconnection",
				Namespaced:   true,
				Kind:         connectionv1alpha1.WorkspaceConnectionKind,
				Verbs:        []string{"create"},
			},
			{
				Name:         "connectionaccessreviews",
				SingularName: "connectionaccessreview",
				Namespaced:   true,
				Kind:         "ConnectionAccessReview",
				Verbs:        []string{"create"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		setupLog.Error(err, "Failed to encode v1 discovery response")
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}

// handleDiscoveryV2 returns discovery in v2 aggregated format
func (s *ExtensionServer) handleDiscoveryV2(w http.ResponseWriter, r *http.Request) {
	response := APIResourceListV2{
		Kind:       "APIResourceList",
		APIVersion: "apidiscovery.k8s.io/v2",
		Metadata:   map[string]interface{}{},
		Resources: []APIResourceV2{
			{
				Resource: "workspaceconnections",
				ResponseKind: ResponseKind{
					Group:   "connection.workspace.jupyter.org",
					Version: "v1alpha1",
					Kind:    connectionv1alpha1.WorkspaceConnectionKind,
				},
				Scope:            "Namespaced",
				SingularResource: "workspaceconnection",
				Verbs:            []string{"create"},
			},
			{
				Resource: "connectionaccessreviews",
				ResponseKind: ResponseKind{
					Group:   "connection.workspace.jupyter.org",
					Version: "v1alpha1",
					Kind:    "ConnectionAccessReview",
				},
				Scope:            "Namespaced",
				SingularResource: "connectionaccessreview",
				Verbs:            []string{"create"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json;g=apidiscovery.k8s.io;v=v2;as=APIResourceList")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		setupLog.Error(err, "Failed to encode v2 discovery response")
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}
