/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"context"
	"fmt"

	dummyv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/dummy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
)

// DummyStorage implements minimal REST storage for DummyResource
// This is a demonstration of InstallAPIGroup with create-only operations
type DummyStorage struct{}

// Ensure DummyStorage implements required interfaces only
var _ rest.Storage = &DummyStorage{}
var _ rest.Scoper = &DummyStorage{}
var _ rest.GroupVersionKindProvider = &DummyStorage{}
var _ rest.Creater = &DummyStorage{}

// NewDummyStorage creates a new DummyStorage
func NewDummyStorage() *DummyStorage {
	return &DummyStorage{}
}

// ============ rest.Storage ============

// New returns a new empty DummyResource object
func (s *DummyStorage) New() runtime.Object {
	return &dummyv1alpha1.DummyResource{}
}

// Destroy cleans up resources on shutdown
func (s *DummyStorage) Destroy() {
	// No cleanup needed for this stateless implementation
}

// ============ rest.Scoper ============

// NamespaceScoped returns true indicating this is a namespaced resource
func (s *DummyStorage) NamespaceScoped() bool {
	return true
}

// ============ rest.GroupVersionKindProvider ============

// GroupVersionKind returns the GVK for DummyResource
func (s *DummyStorage) GroupVersionKind(containingGV schema.GroupVersion) schema.GroupVersionKind {
	return dummyv1alpha1.SchemeGroupVersion.WithKind("DummyResource")
}

// ============ rest.Creater ============

// Create handles POST requests to create a DummyResource
// This is a minimal implementation that returns dummy data without actual storage
func (s *DummyStorage) Create(
	ctx context.Context,
	obj runtime.Object,
	createValidation rest.ValidateObjectFunc,
	options *metav1.CreateOptions,
) (runtime.Object, error) {
	dummy, ok := obj.(*dummyv1alpha1.DummyResource)
	if !ok {
		return nil, fmt.Errorf("invalid object type: expected DummyResource")
	}

	// Run validation
	if err := createValidation(ctx, obj); err != nil {
		return nil, err
	}

	// Set metadata and status - just return dummy data, no actual storage
	now := metav1.Now()
	dummy.CreationTimestamp = now
	dummy.Status.Phase = "Created"
	dummy.Status.LastUpdate = now

	return dummy, nil
}
