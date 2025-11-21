package extensionapi

import (
	"context"
	"fmt"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

// WorkspaceConnectionStorage implements rest.Storage for WorkspaceConnection
type WorkspaceConnectionStorage struct {
	server *ExtensionServer
}

// Ensure WorkspaceConnectionStorage implements required interfaces
var _ rest.Creater = &WorkspaceConnectionStorage{}
var _ rest.Scoper = &WorkspaceConnectionStorage{}
var _ rest.SingularNameProvider = &WorkspaceConnectionStorage{}
var _ rest.Storage = &WorkspaceConnectionStorage{}

// New returns a new WorkspaceConnectionRequest
func (w *WorkspaceConnectionStorage) New() runtime.Object {
	return &connectionv1alpha1.WorkspaceConnectionRequest{}
}

// Destroy cleans up resources on shutdown
func (w *WorkspaceConnectionStorage) Destroy() {
	// Nothing to clean up
}

// NamespaceScoped returns true if the resource is namespaced
func (w *WorkspaceConnectionStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name of the resource
func (w *WorkspaceConnectionStorage) GetSingularName() string {
	return "workspaceconnection"
}

// Create handles POST requests to create a WorkspaceConnection
// This wraps the existing HandleConnectionCreate business logic
func (w *WorkspaceConnectionStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	req, ok := obj.(*connectionv1alpha1.WorkspaceConnectionRequest)
	if !ok {
		return nil, fmt.Errorf("expected WorkspaceConnectionRequest, got %T", obj)
	}

	setupLog.Info("WorkspaceConnectionStorage.Create called",
		"namespace", req.Namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", req.Spec.WorkspaceConnectionType)

	// Validate request
	if err := validateWorkspaceConnectionRequest(req); err != nil {
		setupLog.Error(err, "Invalid workspace connection request")
		return nil, err
	}

	// For now, return a simple response
	// TODO: Integrate with existing HandleConnectionCreate logic
	response := &connectionv1alpha1.WorkspaceConnectionResponse{
		TypeMeta: metav1.TypeMeta{
			APIVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
			Kind:       connectionv1alpha1.WorkspaceConnectionKind,
		},
		ObjectMeta: req.ObjectMeta,
		Spec:       req.Spec,
		Status: connectionv1alpha1.WorkspaceConnectionResponseStatus{
			WorkspaceConnectionType: req.Spec.WorkspaceConnectionType,
			WorkspaceConnectionURL:  "placeholder-url",
		},
	}

	setupLog.Info("WorkspaceConnection created successfully",
		"namespace", req.Namespace,
		"workspaceName", req.Spec.WorkspaceName)

	return response, nil
}
