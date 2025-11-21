package extensionapi

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
)

// DummyStorage implements rest.Storage for DummyResource
type DummyStorage struct{}

// Ensure DummyStorage implements required interfaces
var _ rest.Creater = &DummyStorage{}
var _ rest.Scoper = &DummyStorage{}
var _ rest.SingularNameProvider = &DummyStorage{}
var _ rest.Storage = &DummyStorage{}

// New returns a new DummyResource
func (d *DummyStorage) New() runtime.Object {
	return &DummyResource{}
}

// Destroy cleans up resources on shutdown
func (d *DummyStorage) Destroy() {
	// Nothing to clean up
}

// NamespaceScoped returns true if the resource is namespaced
func (d *DummyStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName returns the singular name of the resource
func (d *DummyStorage) GetSingularName() string {
	return "dummyresource"
}

// Create handles POST requests to create a DummyResource
func (d *DummyStorage) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	dummy := obj.(*DummyResource)
	
	setupLog.Info("DummyStorage.Create called", "name", dummy.Name, "namespace", dummy.Namespace)
	
	// Set some basic metadata
	if dummy.Name == "" {
		dummy.Name = "dummy-resource"
	}
	dummy.UID = "dummy-uid"
	dummy.CreationTimestamp = metav1.Now()
	
	setupLog.Info("DummyResource created successfully", "name", dummy.Name, "uid", dummy.UID)
	return dummy, nil
}
