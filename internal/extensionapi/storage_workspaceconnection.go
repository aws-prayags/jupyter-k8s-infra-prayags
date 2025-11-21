package extensionapi

import (
	"context"
	"fmt"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
)

// WorkspaceConnectionStorage implements rest.Creater for WorkspaceConnection
type WorkspaceConnectionStorage struct {
	// TODO: Add fields needed from current HandleConnectionCreate
	// - k8sClient
	// - sarClient
	// - signerFactory
}

var _ rest.Creater = &WorkspaceConnectionStorage{}
var _ rest.Scoper = &WorkspaceConnectionStorage{}
var _ rest.GroupVersionKindProvider = &WorkspaceConnectionStorage{}
var _ rest.Storage = &WorkspaceConnectionStorage{}

// New returns an empty WorkspaceConnectionResponse object
func (s *WorkspaceConnectionStorage) New() runtime.Object {
	setupLog.Info("WorkspaceConnectionStorage.New() called")
	return &connectionv1alpha1.WorkspaceConnectionResponse{}
}

// NewList returns an empty WorkspaceConnectionResponseList object
func (s *WorkspaceConnectionStorage) NewList() runtime.Object {
	setupLog.Info("WorkspaceConnectionStorage.NewList() called")
	return &connectionv1alpha1.WorkspaceConnectionResponseList{}
}

// GroupVersionKind returns the GVK for WorkspaceConnection
func (s *WorkspaceConnectionStorage) GroupVersionKind(containingGV schema.GroupVersion) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   connectionv1alpha1.GroupName,
		Version: connectionv1alpha1.Version,
		Kind:    connectionv1alpha1.WorkspaceConnectionKind,
	}
}

// Create handles the creation of a WorkspaceConnection
func (s *WorkspaceConnectionStorage) Create(
	ctx context.Context,
	obj runtime.Object,
	createValidation rest.ValidateObjectFunc,
	options *metav1.CreateOptions,
) (runtime.Object, error) {
	setupLog.Info("WorkspaceConnectionStorage.Create() called",
		"objectType", fmt.Sprintf("%T", obj))
	
	req, ok := obj.(*connectionv1alpha1.WorkspaceConnectionRequest)
	if !ok {
		setupLog.Error(nil, "Invalid object type in Create",
			"expected", "*WorkspaceConnectionRequest",
			"got", fmt.Sprintf("%T", obj))
		return nil, errors.NewBadRequest("invalid object type")
	}

	setupLog.Info("Processing WorkspaceConnection create request",
		"workspaceName", req.Spec.WorkspaceName,
		"connectionType", req.Spec.WorkspaceConnectionType,
		"namespace", req.Namespace)

	// TODO: Move actual implementation from HandleConnectionCreate here
	// TODO: This should:
	// 1. Extract user from context (authentication info)
	// 2. Perform SubjectAccessReview for workspace access
	// 3. Generate JWT token with workspace path
	// 4. Build workspaceConnectionUrl with token
	
	// For now, return dummy response
	resp := &connectionv1alpha1.WorkspaceConnectionResponse{
		TypeMeta: metav1.TypeMeta{
			APIVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
			Kind:       connectionv1alpha1.WorkspaceConnectionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              req.Spec.WorkspaceName + "-connection",
			Namespace:         req.Namespace,
			UID:               types.UID("dummy-uid"),
			ResourceVersion:   "1",
			CreationTimestamp: metav1.Now(),
		},
		Spec: req.Spec,
		Status: connectionv1alpha1.WorkspaceConnectionResponseStatus{
			WorkspaceConnectionType: req.Spec.WorkspaceConnectionType,
			WorkspaceConnectionURL:  "https://jupyter.test.com/workspaces/" + req.Namespace + "/" + req.Spec.WorkspaceName + "/bearer-auth?token=DUMMY_TOKEN_TODO",
		},
	}

	setupLog.Info("Successfully created WorkspaceConnection response",
		"name", resp.ObjectMeta.Name,
		"namespace", resp.ObjectMeta.Namespace,
		"connectionUrl", resp.Status.WorkspaceConnectionURL)

	return resp, nil
}

// NamespaceScoped returns true to indicate WorkspaceConnection is namespaced
func (s *WorkspaceConnectionStorage) NamespaceScoped() bool {
	return true
}

// Destroy cleans up resources on shutdown
func (s *WorkspaceConnectionStorage) Destroy() {
	// Nothing to clean up for stateless storage
	setupLog.Info("WorkspaceConnectionStorage.Destroy() called")
}
