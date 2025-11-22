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
	"sync"

	dummyv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/dummy/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var storageLog = log.Log.WithName("dummy-storage")

// DummyStorage implements full CRUD REST storage for DummyResource
// This is a demonstration of InstallAPIGroup with in-memory storage
type DummyStorage struct {
	mu    sync.RWMutex
	items map[string]map[string]*dummyv1alpha1.DummyResource // namespace -> name -> resource
}

// Ensure DummyStorage implements required interfaces
var _ rest.Storage = &DummyStorage{}
var _ rest.Scoper = &DummyStorage{}
var _ rest.GroupVersionKindProvider = &DummyStorage{}
var _ rest.SingularNameProvider = &DummyStorage{}
var _ rest.Creater = &DummyStorage{}
var _ rest.Getter = &DummyStorage{}
var _ rest.Lister = &DummyStorage{}
var _ rest.Updater = &DummyStorage{}
var _ rest.GracefulDeleter = &DummyStorage{}
var _ rest.TableConvertor = &DummyStorage{}

// NewDummyStorage creates a new DummyStorage
func NewDummyStorage() *DummyStorage {
	storageLog.Info("💾 Creating new in-memory DummyStorage")
	return &DummyStorage{
		items: make(map[string]map[string]*dummyv1alpha1.DummyResource),
	}
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

// ============ rest.SingularNameProvider ============

// GetSingularName returns the singular name for the resource
func (s *DummyStorage) GetSingularName() string {
	return "dummyresource"
}

// ============ rest.Creater ============

// Create handles POST requests to create a DummyResource
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

	namespace := dummy.Namespace
	if namespace == "" {
		namespace = "default"
	}

	storageLog.Info("➕ CREATE request", "name", dummy.Name, "namespace", namespace, 
		"workspaceName", dummy.Spec.WorkspaceName, "connectionType", dummy.Spec.WorkspaceConnectionType)

	// Validate required fields
	if dummy.Spec.WorkspaceName == "" {
		err := fmt.Errorf("workspaceName is required")
		storageLog.Error(err, "❌ CREATE validation failed", "name", dummy.Name, "namespace", namespace)
		return nil, err
	}
	if dummy.Spec.WorkspaceConnectionType == "" {
		err := fmt.Errorf("workspaceConnectionType is required")
		storageLog.Error(err, "❌ CREATE validation failed", "name", dummy.Name, "namespace", namespace)
		return nil, err
	}

	// Run validation
	if err := createValidation(ctx, obj); err != nil {
		storageLog.Error(err, "❌ CREATE validation failed", "name", dummy.Name, "namespace", namespace)
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already exists
	if nsItems, ok := s.items[namespace]; ok {
		if _, exists := nsItems[dummy.Name]; exists {
			storageLog.Info("⚠️  CREATE failed: already exists", "name", dummy.Name, "namespace", namespace)
			return nil, errors.NewAlreadyExists(
				schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
				dummy.Name,
			)
		}
	}

	// Initialize namespace map if needed
	if s.items[namespace] == nil {
		s.items[namespace] = make(map[string]*dummyv1alpha1.DummyResource)
	}

	// Set metadata
	now := metav1.Now()
	dummy.CreationTimestamp = now

	// Generate connection URL based on type
	var connectionURL string
	switch dummy.Spec.WorkspaceConnectionType {
	case "vscode-remote":
		connectionURL = fmt.Sprintf("vscode-remote://dummy-connection/%s/%s", 
			namespace, dummy.Spec.WorkspaceName)
		storageLog.Info("🔗 Generated VSCode remote URL", "url", connectionURL)
	case "web-ui":
		connectionURL = fmt.Sprintf("https://jupyter.test.com/workspaces/%s/%s/bearer-auth?token=dummy-token-12345", 
			namespace, dummy.Spec.WorkspaceName)
		storageLog.Info("🔗 Generated Web UI URL", "url", connectionURL)
	default:
		err := fmt.Errorf("invalid workspaceConnectionType: %s (must be 'web-ui' or 'vscode-remote')", 
			dummy.Spec.WorkspaceConnectionType)
		storageLog.Error(err, "❌ CREATE failed", "name", dummy.Name, "namespace", namespace)
		return nil, err
	}

	// Set status with generated URL
	dummy.Status.WorkspaceConnectionType = dummy.Spec.WorkspaceConnectionType
	dummy.Status.WorkspaceConnectionURL = connectionURL

	// Store
	s.items[namespace][dummy.Name] = dummy.DeepCopy()

	storageLog.Info("✅ CREATE successful", "name", dummy.Name, "namespace", namespace, 
		"connectionType", dummy.Status.WorkspaceConnectionType, "url", connectionURL)
	return dummy, nil
}

// ============ rest.Getter ============

// Get handles GET requests for a specific DummyResource
func (s *DummyStorage) Get(
	ctx context.Context,
	name string,
	options *metav1.GetOptions,
) (runtime.Object, error) {
	namespace := "default"
	// Try to get namespace from context (set by API server)
	if ns, ok := ctx.Value("namespace").(string); ok && ns != "" {
		namespace = ns
	}

	storageLog.Info("🔍 GET request", "name", name, "namespace", namespace)

	s.mu.RLock()
	defer s.mu.RUnlock()

	nsItems, ok := s.items[namespace]
	if !ok {
		storageLog.Info("❌ GET failed: not found (namespace empty)", "name", name, "namespace", namespace)
		return nil, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	item, ok := nsItems[name]
	if !ok {
		storageLog.Info("❌ GET failed: not found", "name", name, "namespace", namespace)
		return nil, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	storageLog.Info("✅ GET successful", "name", name, "namespace", namespace, 
		"connectionType", item.Status.WorkspaceConnectionType)
	return item.DeepCopy(), nil
}

// ============ rest.Lister ============

// NewList returns a new empty list object
func (s *DummyStorage) NewList() runtime.Object {
	return &dummyv1alpha1.DummyResourceList{}
}

// List handles LIST requests for DummyResources
func (s *DummyStorage) List(
	ctx context.Context,
	options *metainternalversion.ListOptions,
) (runtime.Object, error) {
	namespace := "default"
	// Try to get namespace from context
	if ns, ok := ctx.Value("namespace").(string); ok && ns != "" {
		namespace = ns
	}

	storageLog.Info("📋 LIST request", "namespace", namespace)

	s.mu.RLock()
	defer s.mu.RUnlock()

	list := &dummyv1alpha1.DummyResourceList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: dummyv1alpha1.SchemeGroupVersion.String(),
			Kind:       "DummyResourceList",
		},
		Items: []dummyv1alpha1.DummyResource{},
	}

	if nsItems, ok := s.items[namespace]; ok {
		for _, item := range nsItems {
			list.Items = append(list.Items, *item.DeepCopy())
		}
	}

	storageLog.Info("✅ LIST successful", "namespace", namespace, "count", len(list.Items))
	return list, nil
}

// ============ rest.Updater ============

// Update handles PUT requests to update a DummyResource
func (s *DummyStorage) Update(
	ctx context.Context,
	name string,
	objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc,
	updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool,
	options *metav1.UpdateOptions,
) (runtime.Object, bool, error) {
	namespace := "default"
	if ns, ok := ctx.Value("namespace").(string); ok && ns != "" {
		namespace = ns
	}

	storageLog.Info("✏️  UPDATE request", "name", name, "namespace", namespace)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get existing object
	nsItems, ok := s.items[namespace]
	if !ok {
		return nil, false, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	existing, ok := nsItems[name]
	if !ok {
		return nil, false, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	// Get updated object
	updated, err := objInfo.UpdatedObject(ctx, existing)
	if err != nil {
		return nil, false, err
	}

	updatedDummy, ok := updated.(*dummyv1alpha1.DummyResource)
	if !ok {
		return nil, false, fmt.Errorf("invalid object type")
	}

	// Validate
	if err := updateValidation(ctx, updatedDummy, existing); err != nil {
		return nil, false, err
	}

	// Store
	s.items[namespace][name] = updatedDummy.DeepCopy()

	storageLog.Info("✅ UPDATE successful", "name", name, "namespace", namespace, 
		"connectionType", updatedDummy.Status.WorkspaceConnectionType)
	return updatedDummy, false, nil
}

// ============ rest.GracefulDeleter ============

// Delete handles DELETE requests for a DummyResource
func (s *DummyStorage) Delete(
	ctx context.Context,
	name string,
	deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions,
) (runtime.Object, bool, error) {
	namespace := "default"
	if ns, ok := ctx.Value("namespace").(string); ok && ns != "" {
		namespace = ns
	}

	storageLog.Info("🗑️  DELETE request", "name", name, "namespace", namespace)

	s.mu.Lock()
	defer s.mu.Unlock()

	nsItems, ok := s.items[namespace]
	if !ok {
		return nil, false, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	item, ok := nsItems[name]
	if !ok {
		return nil, false, errors.NewNotFound(
			schema.GroupResource{Group: dummyv1alpha1.GroupName, Resource: "dummyresources"},
			name,
		)
	}

	// Validate
	if err := deleteValidation(ctx, item); err != nil {
		return nil, false, err
	}

	// Delete
	delete(nsItems, name)

	storageLog.Info("✅ DELETE successful", "name", name, "namespace", namespace)
	return item, true, nil
}

// ============ rest.TableConvertor ============

// ConvertToTable converts objects to table format for kubectl output
func (s *DummyStorage) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Workspace", Type: "string"},
			{Name: "Type", Type: "string"},
			{Name: "URL", Type: "string"},
			{Name: "Age", Type: "string"},
		},
	}

	switch obj := object.(type) {
	case *dummyv1alpha1.DummyResource:
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells: []interface{}{
				obj.Name,
				obj.Spec.WorkspaceName,
				obj.Spec.WorkspaceConnectionType,
				obj.Status.WorkspaceConnectionURL,
				obj.CreationTimestamp.Time,
			},
			Object: runtime.RawExtension{Object: obj},
		})
	case *dummyv1alpha1.DummyResourceList:
		for _, item := range obj.Items {
			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{
					item.Name,
					item.Spec.WorkspaceName,
					item.Spec.WorkspaceConnectionType,
					item.Status.WorkspaceConnectionURL,
					item.CreationTimestamp.Time,
				},
				Object: runtime.RawExtension{Object: &item},
			})
		}
	default:
		return nil, fmt.Errorf("unsupported object type for table conversion")
	}

	return table, nil
}
