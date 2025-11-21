package extensionapi

import (
	"fmt"
	"net/http"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
)

// handleAPIs responds with the list of API groups
func (s *ExtensionServer) handleAPIs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := `{
		"kind": "APIGroupList",
		"apiVersion": "v1",
		"groups": [{
			"name": "connection.workspace.jupyter.org",
			"versions": [{
				"groupVersion": "connection.workspace.jupyter.org/v1alpha1",
				"version": "v1alpha1"
			}],
			"preferredVersion": {
				"groupVersion": "connection.workspace.jupyter.org/v1alpha1",
				"version": "v1alpha1"
			}
		}]
	}`

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write APIs response")
	}
}

// handleAPIGroup responds with the API group information
func (s *ExtensionServer) handleAPIGroup(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := `{
		"kind": "APIGroup",
		"apiVersion": "v1",
		"name": "connection.workspace.jupyter.org",
		"versions": [{
			"groupVersion": "connection.workspace.jupyter.org/v1alpha1",
			"version": "v1alpha1"
		}],
		"preferredVersion": {
			"groupVersion": "connection.workspace.jupyter.org/v1alpha1",
			"version": "v1alpha1"
		}
	}`

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write API group response")
	}
}

// handleDiscovery responds with API resource discovery information
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
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
