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

// handleOpenAPIV3 returns OpenAPI v3 schema for the API group
func (s *ExtensionServer) handleOpenAPIV3(w http.ResponseWriter, r *http.Request) {
	setupLog.Info("OpenAPI v3 endpoint called",
		"path", r.URL.Path,
		"method", r.Method,
		"accept", r.Header.Get("Accept"),
		"userAgent", r.Header.Get("User-Agent"))
	
	schema := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "connection.workspace.jupyter.org",
			"version": "v1alpha1",
		},
		"paths": map[string]interface{}{
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/workspaceconnections": map[string]interface{}{
				"post": map[string]interface{}{
					"description": "Create a WorkspaceConnection",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "namespace",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/connection.workspace.jupyter.org.v1alpha1.WorkspaceConnection",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/connection.workspace.jupyter.org.v1alpha1.WorkspaceConnection",
									},
								},
							},
						},
					},
				},
			},
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/connectionaccessreviews": map[string]interface{}{
				"post": map[string]interface{}{
					"description": "Create a ConnectionAccessReview",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":     "namespace",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/connection.workspace.jupyter.org.v1alpha1.ConnectionAccessReview",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/connection.workspace.jupyter.org.v1alpha1.ConnectionAccessReview",
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"connection.workspace.jupyter.org.v1alpha1.WorkspaceConnection": map[string]interface{}{
					"type":     "object",
					"required": []interface{}{"apiVersion", "kind", "metadata", "spec"},
					"properties": map[string]interface{}{
						"apiVersion": map[string]string{
							"type":        "string",
							"description": "APIVersion defines the versioned schema of this representation",
						},
						"kind": map[string]string{
							"type":        "string",
							"description": "Kind is a string value representing the REST resource",
						},
						"metadata": map[string]interface{}{
							"type":        "object",
							"description": "Standard object metadata",
						},
						"spec": map[string]interface{}{
							"type":        "object",
							"description": "Specification of the desired behavior",
							"required":    []interface{}{"workspaceName", "connectionType"},
							"properties": map[string]interface{}{
								"workspaceName": map[string]string{
									"type":        "string",
									"description": "Name of the workspace",
								},
								"connectionType": map[string]string{
									"type":        "string",
									"description": "Type of connection",
								},
								"connectionDetails": map[string]interface{}{
									"type":        "object",
									"description": "Connection details",
									"additionalProperties": map[string]string{
										"type": "string",
									},
								},
							},
						},
						"status": map[string]interface{}{
							"type":        "object",
							"description": "Most recently observed status",
						},
					},
					"x-kubernetes-group-version-kind": []interface{}{
						map[string]string{
							"group":   "connection.workspace.jupyter.org",
							"kind":    "WorkspaceConnection",
							"version": "v1alpha1",
						},
					},
				},
				"connection.workspace.jupyter.org.v1alpha1.ConnectionAccessReview": map[string]interface{}{
					"type":     "object",
					"required": []interface{}{"apiVersion", "kind", "spec"},
					"properties": map[string]interface{}{
						"apiVersion": map[string]string{
							"type":        "string",
							"description": "APIVersion defines the versioned schema",
						},
						"kind": map[string]string{
							"type":        "string",
							"description": "Kind is a string value representing the REST resource",
						},
						"spec": map[string]interface{}{
							"type":        "object",
							"description": "Specification of the access review request",
							"required":    []interface{}{"workspaceName", "user"},
							"properties": map[string]interface{}{
								"workspaceName": map[string]string{
									"type":        "string",
									"description": "Name of the workspace",
								},
								"user": map[string]string{
									"type":        "string",
									"description": "User requesting access",
								},
							},
						},
						"status": map[string]interface{}{
							"type":        "object",
							"description": "Result of the access review",
							"properties": map[string]interface{}{
								"allowed": map[string]interface{}{
									"type":        "boolean",
									"description": "Whether access is allowed",
								},
								"reason": map[string]string{
									"type":        "string",
									"description": "Reason for the decision",
								},
							},
						},
					},
					"x-kubernetes-group-version-kind": []interface{}{
						map[string]string{
							"group":   "connection.workspace.jupyter.org",
							"kind":    "ConnectionAccessReview",
							"version": "v1alpha1",
						},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(schema); err != nil {
		setupLog.Error(err, "Failed to encode OpenAPI v3 response")
		WriteError(w, http.StatusInternalServerError, "failed to write OpenAPI v3 schema")
		return
	}
	
	setupLog.Info("Successfully returned OpenAPI v3 schema",
		"schemaVersion", "3.0.0",
		"resourceCount", 2)
}
