package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
)

// handleDiscovery responds with API resource discovery information
// Supports both v1 and v2 discovery formats based on Accept header
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	acceptHeader := r.Header.Get("Accept")
	
	// Log the request for debugging
	setupLog.Info("Discovery endpoint called",
		"path", r.URL.Path,
		"accept", acceptHeader,
		"method", r.Method)
	
	// Check if client wants v2 aggregated discovery
	if strings.Contains(acceptHeader, "application/json;g=apidiscovery.k8s.io;v=v2beta1") ||
		strings.Contains(acceptHeader, "application/json;g=apidiscovery.k8s.io;v=v2") {
		setupLog.Info("Returning v2 aggregated discovery format")
		s.handleV2Discovery(w, r)
		return
	}
	
	// Default to v1 format
	setupLog.Info("Returning v1 discovery format")
	s.handleV1Discovery(w, r)
}

// handleV1Discovery returns v1 APIResourceList format
func (s *ExtensionServer) handleV1Discovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "%s",
		"resources": [{
			"name": "workspaceconnections",
			"singularName": "workspaceconnection",
			"namespaced": true,
			"kind": "%s",
			"verbs": ["create"]
		}, {
			"name": "connectionaccessreviews",
			"singularName": "connectionaccessreview",
			"namespaced": true,
			"kind": "ConnectionAccessReview",
			"verbs": ["create"]
		}]
	}`, connectionv1alpha1.WorkspaceConnectionAPIVersion, connectionv1alpha1.WorkspaceConnectionKind)

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}

// handleV2Discovery returns v2 aggregated discovery format
func (s *ExtensionServer) handleV2Discovery(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"kind":       "APIGroupDiscoveryList",
		"apiVersion": "apidiscovery.k8s.io/v2beta1",
		"metadata":   map[string]interface{}{},
		"items": []map[string]interface{}{
			{
				"metadata": map[string]interface{}{
					"name":              "connection.workspace.jupyter.org",
					"creationTimestamp": nil,
				},
				"versions": []map[string]interface{}{
					{
						"version": "v1alpha1",
						"resources": []map[string]interface{}{
							{
								"resource": "workspaceconnections",
								"responseKind": map[string]interface{}{
									"group":   "connection.workspace.jupyter.org",
									"version": "v1alpha1",
									"kind":    connectionv1alpha1.WorkspaceConnectionKind,
								},
								"scope":            "Namespaced",
								"singularResource": "workspaceconnection",
								"verbs":            []string{"create"},
								"shortNames":       []string{},
								"categories":       []string{},
							},
							{
								"resource": "connectionaccessreviews",
								"responseKind": map[string]interface{}{
									"group":   "connection.workspace.jupyter.org",
									"version": "v1alpha1",
									"kind":    "ConnectionAccessReview",
								},
								"scope":            "Namespaced",
								"singularResource": "connectionaccessreview",
								"verbs":            []string{"create"},
								"shortNames":       []string{},
								"categories":       []string{},
							},
						},
						"freshness": "Current",
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json;g=apidiscovery.k8s.io;v=v2beta1;as=APIGroupDiscoveryList")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		setupLog.Error(err, "Failed to encode v2 discovery response")
		WriteError(w, http.StatusInternalServerError, "failed to write v2 discovery body")
	}
	
	setupLog.Info("Successfully returned v2 discovery response")
}
