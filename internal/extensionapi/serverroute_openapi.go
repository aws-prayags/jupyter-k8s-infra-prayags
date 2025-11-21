/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"net/http"
)

// handleOpenAPIv2 serves the OpenAPI v2 specification
// This endpoint is required by the Kubernetes aggregation layer for discovery and structural validation.
func (s *ExtensionServer) handleOpenAPIv2(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Complete OpenAPI v2 spec including all REST verbs for WorkspaceConnection resource
	// This structural spec includes necessary core K8s definitions (like ObjectMeta)
	// to ensure proper validation, defaulting, and pruning by the Kubernetes aggregation layer.
	response := `{
		"swagger": "2.0",
		"info": {
			"title": "connection.workspace.jupyter.org",
			"version": "v1alpha1"
		},
		"paths": {
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/workspaceconnections": {
				"get": {
					"description": "list or watch objects of kind WorkspaceConnection",
					"operationId": "listConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/namespace"}
					],
					"responses": {
						"200": {
							"description": "OK",
							"schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnectionList"}
						}
					}
				},
				"post": {
					"description": "create a WorkspaceConnection",
					"operationId": "createConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/namespace"},
						{
							"name": "body",
							"in": "body",
							"required": true,
							"schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}
						}
					],
					"responses": {
						"200": {"description": "OK", "schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}}
					}
				},
				"delete": {
					"description": "delete collection of WorkspaceConnection",
					"operationId": "deleteCollectionConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/namespace"}
					],
					"responses": {
						"200": {"description": "OK"}
					}
				}
			},
			"/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/workspaceconnections/{name}": {
				"get": {
					"description": "read the specified WorkspaceConnection",
					"operationId": "readConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/name"},
						{"$ref": "#/parameters/namespace"}
					],
					"responses": {
						"200": {"description": "OK", "schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}}
					}
				},
				"put": {
					"description": "replace the specified WorkspaceConnection",
					"operationId": "replaceConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/name"},
						{"$ref": "#/parameters/namespace"},
						{
							"name": "body",
							"in": "body",
							"required": true,
							"schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}
						}
					],
					"responses": {
						"200": {"description": "OK", "schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}}
					}
				},
				"patch": {
					"description": "partially update the specified WorkspaceConnection",
					"operationId": "patchConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/name"},
						{"$ref": "#/parameters/namespace"}
					],
					"responses": {
						"200": {"description": "OK", "schema": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}}
					}
				},
				"delete": {
					"description": "delete a WorkspaceConnection",
					"operationId": "deleteConnectionWorkspaceJupyterOrgV1alpha1NamespacedWorkspaceConnection",
					"parameters": [
						{"$ref": "#/parameters/name"},
						{"$ref": "#/parameters/namespace"}
					],
					"responses": {
						"200": {"description": "OK"}
					}
				}
			}
		},
		"definitions": {
			"v1.ObjectMeta": {
				"description": "ObjectMeta is metadata that all persisted resources must have, which includes all objects users must create.",
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Name must be unique within a namespace."},
					"namespace": {"type": "string", "description": "Namespace defines the space within which each name must be unique."},
					"labels": {"type": "object", "additionalProperties": {"type": "string"}},
					"annotations": {"type": "object", "additionalProperties": {"type": "string"}},
					"resourceVersion": {"type": "string"}
				}
			},
			"v1.ListMeta": {
				"description": "ListMeta describes metadata that synthetic resources must have, including lists and various status objects. A resource may have only one of {ObjectMeta, ListMeta}.",
				"type": "object",
				"properties": {
					"resourceVersion": {"type": "string"},
					"continue": {"type": "string"}
				}
			},
			"v1alpha1.WorkspaceConnection": {
				"description": "WorkspaceConnection is the Schema for the workspaceconnections API",
				"type": "object",
				"x-kubernetes-preserve-unknown-fields": true,
				"properties": {
					"apiVersion": {"type": "string"},
					"kind": {"type": "string"},
					"metadata": {"$ref": "#/definitions/v1.ObjectMeta"},
					"spec": {
						"type": "object",
						"x-kubernetes-preserve-unknown-fields": true,
						"properties": {
							"workspaceName": {"type": "string"},
							"workspaceConnectionType": {"type": "string"}
						}
					},
					"status": {
						"type": "object",
						"x-kubernetes-preserve-unknown-fields": true,
						"properties": {}
					}
				}
			},
			"v1alpha1.WorkspaceConnectionList": {
				"description": "WorkspaceConnectionList contains a list of WorkspaceConnection",
				"type": "object",
				"properties": {
					"apiVersion": {"type": "string"},
					"kind": {"type": "string"},
					"metadata": {"$ref": "#/definitions/v1.ListMeta"},
					"items": {
						"type": "array",
						"items": {"$ref": "#/definitions/v1alpha1.WorkspaceConnection"}
					}
				}
			}
		},
		"parameters": {
			"name": {
				"name": "name",
				"in": "path",
				"description": "name of the WorkspaceConnection",
				"required": true,
				"type": "string"
			},
			"namespace": {
				"name": "namespace",
				"in": "path",
				"description": "object name and auth scope, such as for entire cluster",
				"required": true,
				"type": "string"
			}
		}
	}`

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write OpenAPI v2 response")
	}
}
// This tells Kubernetes where to find the actual v3 spec.
func (s *ExtensionServer) handleOpenAPIv3Discovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Discovery response pointing to the actual spec endpoint
	response := `{
		"paths": {
			"/openapi/v3/apis/connection.workspace.jupyter.org/v1alpha1": {
				"serverRelativeURL": "/openapi/v3/apis/connection.workspace.jupyter.org/v1alpha1"
			}
		}
	}`

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write OpenAPI v3 discovery response")
	}
}

// handleOpenAPIv3Spec serves the actual OpenAPI v3 specification, including structural schema.
func (s *ExtensionServer) handleOpenAPIv3Spec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Full OpenAPI v3 structural spec to satisfy Kubernetes aggregation controller
	// Uses components/schemas instead of definitions
	response := `{
		"openapi": "3.0.0",
		"info": {
			"title": "connection.workspace.jupyter.org",
			"version": "v1alpha1"
		},
		"paths": {},
		"components": {
			"schemas": {
				"v1.ObjectMeta": {
					"description": "ObjectMeta is metadata that all persisted resources must have, which includes all objects users must create.",
					"type": "object",
					"properties": {
						"name": {"type": "string", "description": "Name must be unique within a namespace."},
						"namespace": {"type": "string", "description": "Namespace defines the space within which each name must be unique."},
						"labels": {"type": "object", "additionalProperties": {"type": "string"}},
						"annotations": {"type": "object", "additionalProperties": {"type": "string"}},
						"resourceVersion": {"type": "string"}
					}
				},
				"v1.ListMeta": {
					"description": "ListMeta describes metadata that synthetic resources must have, including lists and various status objects. A resource may have only one of {ObjectMeta, ListMeta}.",
					"type": "object",
					"properties": {
						"resourceVersion": {"type": "string"},
						"continue": {"type": "string"}
					}
				},
				"v1alpha1.WorkspaceConnection": {
					"description": "WorkspaceConnection is the Schema for the workspaceconnections API",
					"type": "object",
					"x-kubernetes-preserve-unknown-fields": true,
					"properties": {
						"apiVersion": {"type": "string"},
						"kind": {"type": "string"},
						"metadata": {"$ref": "#/components/schemas/v1.ObjectMeta"},
						"spec": {
							"type": "object",
							"x-kubernetes-preserve-unknown-fields": true,
							"properties": {
								"workspaceName": {"type": "string"},
								"workspaceConnectionType": {"type": "string"}
							}
						},
						"status": {
							"type": "object",
							"x-kubernetes-preserve-unknown-fields": true,
							"properties": {}
						}
					}
				},
				"v1alpha1.WorkspaceConnectionList": {
					"description": "WorkspaceConnectionList contains a list of WorkspaceConnection",
					"type": "object",
					"properties": {
						"apiVersion": {"type": "string"},
						"kind": {"type": "string"},
						"metadata": {"$ref": "#/components/schemas/v1.ListMeta"},
						"items": {
							"type": "array",
							"items": {"$ref": "#/components/schemas/v1alpha1.WorkspaceConnection"}
						}
					}
				}
			}
		}
	}`

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write OpenAPI v3 spec response")
	}
}
