package extensionapi

import (
	"encoding/json"
	"net/http"
)

// handleOpenAPIRoot returns the OpenAPI v3 root document listing available API groups
func (s *ExtensionServer) handleOpenAPIRoot(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"paths": map[string]interface{}{
			"apis/connection.workspace.jupyter.org/v1alpha1": map[string]string{
				"serverRelativeURL": "/openapi/v3/apis/connection.workspace.jupyter.org/v1alpha1",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleOpenAPISpec returns the OpenAPI v3 spec for connection.workspace.jupyter.org/v1alpha1
func (s *ExtensionServer) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "connection.workspace.jupyter.org",
			"version": "v1alpha1",
		},
		"paths": map[string]interface{}{
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/workspaceconnections": map[string]interface{}{
				"post": map[string]interface{}{
					"description": "create a WorkspaceConnection",
					"operationId": "createWorkspaceConnection",
					"parameters": []map[string]interface{}{
						{
							"name":     "namespace",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{
									"$ref": "#/components/schemas/WorkspaceConnection",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Created",
						},
					},
				},
			},
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/connectionaccessreviews": map[string]interface{}{
				"post": map[string]interface{}{
					"description": "create a ConnectionAccessReview",
					"operationId": "createConnectionAccessReview",
					"parameters": []map[string]interface{}{
						{
							"name":     "namespace",
							"in":       "path",
							"required": true,
							"schema":   map[string]string{"type": "string"},
						},
					},
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{
									"$ref": "#/components/schemas/ConnectionAccessReview",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "Created",
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"WorkspaceConnection": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"apiVersion": map[string]string{
							"type": "string",
						},
						"kind": map[string]string{
							"type": "string",
						},
						"metadata": map[string]interface{}{
							"type": "object",
						},
						"spec": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"workspaceName": map[string]string{
									"type": "string",
								},
								"workspaceConnectionType": map[string]string{
									"type": "string",
								},
							},
						},
					},
				},
				"ConnectionAccessReview": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"apiVersion": map[string]string{
							"type": "string",
						},
						"kind": map[string]string{
							"type": "string",
						},
						"metadata": map[string]interface{}{
							"type": "object",
						},
						"spec": map[string]interface{}{
							"type": "object",
						},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(spec)
}
