package extensionapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DummyResource is a minimal resource to test GenericAPIServer integration
type DummyResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// DeepCopyObject implements runtime.Object
func (d *DummyResource) DeepCopyObject() runtime.Object {
	if d == nil {
		return nil
	}
	out := new(DummyResource)
	d.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out
func (d *DummyResource) DeepCopyInto(out *DummyResource) {
	*out = *d
	out.TypeMeta = d.TypeMeta
	d.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
}

// GetObjectKind implements runtime.Object
func (d *DummyResource) GetObjectKind() schema.ObjectKind {
	return &d.TypeMeta
}

// DummyResourceList is a list of DummyResources
type DummyResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DummyResource `json:"items"`
}

// DeepCopyObject implements runtime.Object
func (d *DummyResourceList) DeepCopyObject() runtime.Object {
	if d == nil {
		return nil
	}
	out := new(DummyResourceList)
	d.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out
func (d *DummyResourceList) DeepCopyInto(out *DummyResourceList) {
	*out = *d
	out.TypeMeta = d.TypeMeta
	out.ListMeta = d.ListMeta
	if d.Items != nil {
		out.Items = make([]DummyResource, len(d.Items))
		for i := range d.Items {
			d.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// GetObjectKind implements runtime.Object
func (d *DummyResourceList) GetObjectKind() schema.ObjectKind {
	return &d.TypeMeta
}
