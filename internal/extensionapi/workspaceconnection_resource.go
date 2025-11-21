package extensionapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// WorkspaceConnection is a minimal resource for workspace connections
type WorkspaceConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceConnectionSpec   `json:"spec,omitempty"`
	Status            WorkspaceConnectionStatus `json:"status,omitempty"`
}

type WorkspaceConnectionSpec struct {
	WorkspaceName           string `json:"workspaceName"`
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
}

// DeepCopyObject implements runtime.Object
func (w *WorkspaceConnection) DeepCopyObject() runtime.Object {
	if w == nil {
		return nil
	}
	out := new(WorkspaceConnection)
	w.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out
func (w *WorkspaceConnection) DeepCopyInto(out *WorkspaceConnection) {
	*out = *w
	out.TypeMeta = w.TypeMeta
	w.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = w.Spec
	out.Status = w.Status
}

// GetObjectKind implements runtime.Object
func (w *WorkspaceConnection) GetObjectKind() schema.ObjectKind {
	return &w.TypeMeta
}

// WorkspaceConnectionResponse is the response resource
type WorkspaceConnectionResponse struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceConnectionSpec        `json:"spec,omitempty"`
	Status            WorkspaceConnectionStatus      `json:"status,omitempty"`
}

type WorkspaceConnectionStatus struct {
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
	WorkspaceConnectionURL  string `json:"workspaceConnectionUrl"`
}

// DeepCopyObject implements runtime.Object
func (w *WorkspaceConnectionResponse) DeepCopyObject() runtime.Object {
	if w == nil {
		return nil
	}
	out := new(WorkspaceConnectionResponse)
	w.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out
func (w *WorkspaceConnectionResponse) DeepCopyInto(out *WorkspaceConnectionResponse) {
	*out = *w
	out.TypeMeta = w.TypeMeta
	w.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = w.Spec
	out.Status = w.Status
}

// GetObjectKind implements runtime.Object
func (w *WorkspaceConnectionResponse) GetObjectKind() schema.ObjectKind {
	return &w.TypeMeta
}
