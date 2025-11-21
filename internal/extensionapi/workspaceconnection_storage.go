package extensionapi

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

type WorkspaceConnectionStorage struct {
	server *ExtensionServer
}

var _ rest.Creater = &WorkspaceConnectionStorage{}
var _ rest.Scoper = &WorkspaceConnectionStorage{}
var _ rest.SingularNameProvider = &WorkspaceConnectionStorage{}
var _ rest.Storage = &WorkspaceConnectionStorage{}

func (w *WorkspaceConnectionStorage) New() runtime.Object {
	return &WorkspaceConnection{}
}

func (w *WorkspaceConnectionStorage) Destroy() {}

func (w *WorkspaceConnectionStorage) NamespaceScoped() bool {
	return true
}

func (w *WorkspaceConnectionStorage) GetSingularName() string {
	return "workspaceconnection"
}

func (w *WorkspaceConnectionStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	req, ok := obj.(*WorkspaceConnection)
	if !ok {
		return nil, fmt.Errorf("expected WorkspaceConnection, got %T", obj)
	}

	// Validate request
	if req.Spec.WorkspaceName == "" {
		return nil, fmt.Errorf("workspaceName is required")
	}
	if req.Spec.WorkspaceConnectionType == "" {
		return nil, fmt.Errorf("workspaceConnectionType is required")
	}

	// Set response metadata
	req.TypeMeta = metav1.TypeMeta{
		APIVersion: "connection.workspace.jupyter.org/v1alpha1",
		Kind:       "WorkspaceConnection",
	}
	
	// Extract user from Kubernetes context
	var user string
	if userInfo, ok := request.UserFrom(ctx); ok && userInfo != nil {
		user = userInfo.GetName()
	}
	if user == "" {
		return nil, fmt.Errorf("user information not found in request context")
	}

	// Generate URL using existing business logic
	var connectionType, connectionURL string
	var err error
	
	if w.server != nil {
		namespace := req.Namespace
		if namespace == "" {
			namespace = "default"
		}
		
		switch req.Spec.WorkspaceConnectionType {
		case "vscode-remote":
			connectionType, connectionURL, err = w.server.generateVSCodeURL(ctx, user, req.Spec.WorkspaceName, namespace)
		case "web-ui":
			connectionType, connectionURL, err = w.server.generateWebUIBearerTokenURL(ctx, user, req.Spec.WorkspaceName, namespace)
		default:
			err = fmt.Errorf("invalid workspaceConnectionType: %s", req.Spec.WorkspaceConnectionType)
		}
		
		if err != nil {
			return nil, fmt.Errorf("failed to generate connection URL: %w", err)
		}
	} else {
		connectionType = req.Spec.WorkspaceConnectionType
		connectionURL = "placeholder-url"
	}
	
	// Add status with connection details
	req.Status = WorkspaceConnectionStatus{
		WorkspaceConnectionType: connectionType,
		WorkspaceConnectionURL:  connectionURL,
	}

	return req, nil
}
