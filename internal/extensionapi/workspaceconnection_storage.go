package extensionapi

import (
	"context"
	"fmt"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
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

// New returns a new WorkspaceConnection (the kind used in kubectl commands)
func (w *WorkspaceConnectionStorage) New() runtime.Object {
	return &connectionv1alpha1.WorkspaceConnection{}
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
	// Accept both WorkspaceConnection and WorkspaceConnectionRequest for backward compatibility
	var spec connectionv1alpha1.WorkspaceConnectionRequestSpec
	var metadata metav1.ObjectMeta
	
	switch v := obj.(type) {
	case *connectionv1alpha1.WorkspaceConnection:
		spec = v.Spec
		metadata = v.ObjectMeta
	case *connectionv1alpha1.WorkspaceConnectionRequest:
		spec = v.Spec
		metadata = v.ObjectMeta
	default:
		return nil, fmt.Errorf("expected WorkspaceConnection or WorkspaceConnectionRequest, got %T", obj)
	}
	
	// Create a request object for internal processing
	req := &connectionv1alpha1.WorkspaceConnectionRequest{
		ObjectMeta: metadata,
		Spec:       spec,
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

	// Check if CLUSTER_ID is configured for VSCode connections
	if req.Spec.WorkspaceConnectionType == connectionv1alpha1.ConnectionTypeVSCodeRemote {
		if w.server.config.ClusterId == "" {
			return nil, fmt.Errorf("CLUSTER_ID not configured. Please set controllerManager.container.env.CLUSTER_ID in helm values")
		}
	}

	// Get user from context using GenericAPIServer's standard method
	user := "system:anonymous" // Default
	if userInfo, ok := request.UserFrom(ctx); ok {
		user = userInfo.GetName()
		setupLog.Info("Extracted user from context", "user", user)
	} else {
		setupLog.Info("Could not determine user from context, using default", "user", user)
	}

	// Check authorization for private workspaces
	result, err := w.server.CheckWorkspaceAccess(req.Namespace, req.Spec.WorkspaceName, user, w.server.logger)
	if err != nil {
		setupLog.Error(err, "Authorization failed", "workspaceName", req.Spec.WorkspaceName)
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	if result.NotFound {
		return nil, fmt.Errorf("workspace not found: %s", result.Reason)
	}

	if !result.Allowed {
		return nil, fmt.Errorf("access forbidden: %s", result.Reason)
	}

	// Generate connection URL based on type
	var responseType, responseURL string
	switch req.Spec.WorkspaceConnectionType {
	case connectionv1alpha1.ConnectionTypeVSCodeRemote:
		responseType, responseURL, err = w.generateVSCodeURLFromStorage(ctx, req.Spec.WorkspaceName, req.Namespace)
	case connectionv1alpha1.ConnectionTypeWebUI:
		responseType, responseURL, err = w.generateWebUIURLFromStorage(ctx, req.Spec.WorkspaceName, req.Namespace, user)
	default:
		return nil, fmt.Errorf("invalid workspace connection type: %s", req.Spec.WorkspaceConnectionType)
	}

	if err != nil {
		setupLog.Error(err, "Failed to generate connection URL", "connectionType", req.Spec.WorkspaceConnectionType)
		return nil, fmt.Errorf("failed to generate connection URL: %w", err)
	}

	// Create response
	response := &connectionv1alpha1.WorkspaceConnectionResponse{
		TypeMeta: metav1.TypeMeta{
			APIVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
			Kind:       connectionv1alpha1.WorkspaceConnectionKind,
		},
		ObjectMeta: req.ObjectMeta,
		Spec:       req.Spec,
		Status: connectionv1alpha1.WorkspaceConnectionResponseStatus{
			WorkspaceConnectionType: responseType,
			WorkspaceConnectionURL:  responseURL,
		},
	}

	setupLog.Info("WorkspaceConnection created successfully",
		"namespace", req.Namespace,
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", responseType)

	return response, nil
}

// generateVSCodeURLFromStorage generates VSCode URL (extracted from server method)
func (w *WorkspaceConnectionStorage) generateVSCodeURLFromStorage(ctx context.Context, workspaceName, namespace string) (string, string, error) {
	return w.server.generateVSCodeURLInternal(ctx, workspaceName, namespace)
}

// generateWebUIURLFromStorage generates WebUI URL (extracted from server method)
func (w *WorkspaceConnectionStorage) generateWebUIURLFromStorage(ctx context.Context, workspaceName, namespace, user string) (string, string, error) {
	return w.server.generateWebUIURLInternal(ctx, workspaceName, namespace, user)
}
