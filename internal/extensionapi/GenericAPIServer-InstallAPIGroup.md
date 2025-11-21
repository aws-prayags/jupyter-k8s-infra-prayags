# Extension API Server with InstallAPIGroup

An alternative implementation approach using Kubernetes' `InstallAPIGroup` for full REST storage semantics.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Controller Manager                           │
│                        (cmd/main.go)                                 │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │  --enable-extension-api │
                    └────────────┬────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│              Extension API Server Setup                              │
│         (extensionapi.SetupExtensionAPIServerWithManager)            │
│                                                                       │
│  1. Register Scheme (WorkspaceConnection types)                     │
│  2. Create Codec Factory                                            │
│  3. Create GenericAPIServer with RecommendedOptions                 │
│  4. Create REST Storage implementations                             │
│  5. Install API Group with storage                                  │
│  6. Add to Manager as Runnable                                      │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    GenericAPIServer                                  │
│              (k8s.io/apiserver/pkg/server)                          │
│                                                                       │
│  • TLS/HTTPS (port 7443)                                            │
│  • Authentication (via RecommendedOptions)                          │
│  • Automatic API discovery registration                             │
│  • Built-in CRUD routing                                            │
│  • OpenAPI schema generation                                        │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    InstallAPIGroup                                   │
│              (k8s.io/apiserver/pkg/server)                          │
│                                                                       │
│  Automatically registers:                                            │
│  • /apis/connection.workspace.jupyter.org                           │
│  • /apis/connection.workspace.jupyter.org/v1alpha1                  │
│  • /apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{ns}/workspaceconnections
│  • Full CRUD endpoints (GET, LIST, CREATE, UPDATE, DELETE, WATCH)  │
│  • OpenAPI spec generation                                          │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    REST Storage Layer                                │
│              (custom implementation)                                 │
│                                                                       │
│  WorkspaceConnectionStorage implements:                              │
│  • rest.Storage                                                      │
│  • rest.Creater                                                      │
│  • rest.Scoper (namespaced)                                         │
│  • rest.GroupVersionKindProvider                                    │
│                                                                       │
│  Optional interfaces:                                                │
│  • rest.Getter (for GET operations)                                 │
│  • rest.Lister (for LIST operations)                                │
│  • rest.Updater (for UPDATE operations)                             │
│  • rest.GracefulDeleter (for DELETE operations)                     │
│  • rest.Watcher (for WATCH operations)                              │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Differences from PathRecorderMux

| Aspect | InstallAPIGroup | PathRecorderMux |
|--------|----------------|-----------------|
| **Routing** | Automatic CRUD routes | Manual route registration |
| **Discovery** | Auto-registered at `/apis` | Manual JSON response |
| **Storage** | Required (REST storage interface) | Not needed |
| **CRUD Operations** | Full support (GET, LIST, CREATE, UPDATE, DELETE, WATCH) | Only what you implement |
| **OpenAPI** | Auto-generated | Manual if needed |
| **Scheme** | Required registration | Not required |
| **Codec** | Required for serialization | Manual JSON handling |
| **Complexity** | Higher (more boilerplate) | Lower (direct handlers) |
| **Use Case** | Resource-oriented APIs | RPC-style operations |

## Implementation Structure

### 1. Scheme Registration

```go
import (
    connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
    scheme = runtime.NewScheme()
    codecs serializer.CodecFactory
)

func init() {
    // Register WorkspaceConnection types
    connectionv1alpha1.AddToScheme(scheme)
    
    // Create codec factory for serialization/deserialization
    codecs = serializer.NewCodecFactory(scheme)
}
```

### 2. REST Storage Implementation

```go
// WorkspaceConnectionStorage implements REST storage for WorkspaceConnection
type WorkspaceConnectionStorage struct {
    k8sClient     client.Client
    sarClient     v1.SubjectAccessReviewInterface
    signerFactory jwt.SignerFactory
    config        *ExtensionConfig
}

// Implement required interfaces
var _ rest.Storage = &WorkspaceConnectionStorage{}
var _ rest.Creater = &WorkspaceConnectionStorage{}
var _ rest.Scoper = &WorkspaceConnectionStorage{}
var _ rest.GroupVersionKindProvider = &WorkspaceConnectionStorage{}

// New returns the resource type
func (s *WorkspaceConnectionStorage) New() runtime.Object {
    return &connectionv1alpha1.WorkspaceConnectionRequest{}
}

// Destroy cleans up resources
func (s *WorkspaceConnectionStorage) Destroy() {}

// NamespaceScoped returns true (this is a namespaced resource)
func (s *WorkspaceConnectionStorage) NamespaceScoped() bool {
    return true
}

// GroupVersionKind returns the GVK
func (s *WorkspaceConnectionStorage) GroupVersionKind(containingGV schema.GroupVersion) schema.GroupVersionKind {
    return schema.GroupVersionKind{
        Group:   "connection.workspace.jupyter.org",
        Version: "v1alpha1",
        Kind:    "WorkspaceConnection",
    }
}

// Create handles POST requests
func (s *WorkspaceConnectionStorage) Create(
    ctx context.Context,
    obj runtime.Object,
    createValidation rest.ValidateObjectFunc,
    options *metav1.CreateOptions,
) (runtime.Object, error) {
    // Extract request
    req, ok := obj.(*connectionv1alpha1.WorkspaceConnectionRequest)
    if !ok {
        return nil, fmt.Errorf("invalid object type")
    }
    
    // Validate
    if err := createValidation(ctx, obj); err != nil {
        return nil, err
    }
    
    // Get user from context
    user, ok := request.UserFrom(ctx)
    if !ok {
        return nil, fmt.Errorf("user not found in context")
    }
    
    // Get namespace from context
    namespace, ok := request.NamespaceFrom(ctx)
    if !ok {
        return nil, fmt.Errorf("namespace not found in context")
    }
    
    // Check authorization
    result, err := s.checkWorkspaceAuthorization(ctx, req.Spec.WorkspaceName, namespace, user.GetName())
    if err != nil {
        return nil, err
    }
    if !result.Allowed {
        return nil, errors.NewForbidden(
            schema.GroupResource{Group: "connection.workspace.jupyter.org", Resource: "workspaceconnections"},
            req.Name,
            fmt.Errorf(result.Reason),
        )
    }
    
    // Generate connection URL
    var connectionType, connectionURL string
    switch req.Spec.WorkspaceConnectionType {
    case connectionv1alpha1.ConnectionTypeVSCodeRemote:
        connectionType, connectionURL, err = s.generateVSCodeURL(ctx, req.Spec.WorkspaceName, namespace)
    case connectionv1alpha1.ConnectionTypeWebUI:
        connectionType, connectionURL, err = s.generateWebUIURL(ctx, req.Spec.WorkspaceName, namespace, user.GetName())
    default:
        return nil, errors.NewBadRequest("invalid workspace connection type")
    }
    
    if err != nil {
        return nil, errors.NewInternalError(err)
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
            WorkspaceConnectionType: connectionType,
            WorkspaceConnectionURL:  connectionURL,
        },
    }
    
    return response, nil
}
```

### 3. Optional: GET/LIST Storage (if needed)

```go
// Implement rest.Getter for GET operations
var _ rest.Getter = &WorkspaceConnectionStorage{}

func (s *WorkspaceConnectionStorage) Get(
    ctx context.Context,
    name string,
    options *metav1.GetOptions,
) (runtime.Object, error) {
    // For ephemeral resources, you might return NotFound
    // or implement a cache/lookup mechanism
    return nil, errors.NewNotFound(
        schema.GroupResource{Group: "connection.workspace.jupyter.org", Resource: "workspaceconnections"},
        name,
    )
}

// Implement rest.Lister for LIST operations
var _ rest.Lister = &WorkspaceConnectionStorage{}

func (s *WorkspaceConnectionStorage) NewList() runtime.Object {
    return &connectionv1alpha1.WorkspaceConnectionList{}
}

func (s *WorkspaceConnectionStorage) List(
    ctx context.Context,
    options *internalversion.ListOptions,
) (runtime.Object, error) {
    // Return empty list for ephemeral resources
    return &connectionv1alpha1.WorkspaceConnectionList{
        TypeMeta: metav1.TypeMeta{
            APIVersion: connectionv1alpha1.WorkspaceConnectionAPIVersion,
            Kind:       "WorkspaceConnectionList",
        },
        Items: []connectionv1alpha1.WorkspaceConnectionResponse{},
    }, nil
}
```

### 4. Installing the API Group

```go
func SetupExtensionAPIServerWithManager(mgr ctrl.Manager, config *ExtensionConfig) error {
    // Create GenericAPIServer
    recommendedOptions := createRecommendedOptions(config)
    serverConfig := genericapiserver.NewRecommendedConfig(codecs)
    serverConfig.EffectiveVersion = compatibility.DefaultBuildEffectiveVersion()
    
    if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
        return err
    }
    
    genericServer, err := serverConfig.Complete().New("extension-apiserver", genericapiserver.NewEmptyDelegate())
    if err != nil {
        return err
    }
    
    // Create REST storage
    storage := &WorkspaceConnectionStorage{
        k8sClient:     mgr.GetClient(),
        sarClient:     sarClient,
        signerFactory: signerFactory,
        config:        config,
    }
    
    // Create API group info
    apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
        "connection.workspace.jupyter.org",
        scheme,
        metav1.ParameterCodec,
        codecs,
    )
    
    // Register storage for v1alpha1
    v1alpha1Storage := map[string]rest.Storage{
        "workspaceconnections": storage,
    }
    apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1Storage
    
    // Install API group
    if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
        return fmt.Errorf("failed to install API group: %w", err)
    }
    
    // Add to manager
    return mgr.Add(&extensionServerRunnable{genericServer: genericServer})
}
```

## Automatic API Discovery

With `InstallAPIGroup`, discovery is automatic:

### Group Discovery

```bash
GET /apis
```

Returns:
```json
{
  "kind": "APIGroupList",
  "apiVersion": "v1",
  "groups": [
    {
      "name": "connection.workspace.jupyter.org",
      "versions": [
        {
          "groupVersion": "connection.workspace.jupyter.org/v1alpha1",
          "version": "v1alpha1"
        }
      ],
      "preferredVersion": {
        "groupVersion": "connection.workspace.jupyter.org/v1alpha1",
        "version": "v1alpha1"
      }
    }
  ]
}
```

### Resource Discovery

```bash
GET /apis/connection.workspace.jupyter.org/v1alpha1
```

Returns:
```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "connection.workspace.jupyter.org/v1alpha1",
  "resources": [
    {
      "name": "workspaceconnections",
      "singularName": "workspaceconnection",
      "namespaced": true,
      "kind": "WorkspaceConnection",
      "verbs": ["create", "get", "list"],
      "categories": []
    }
  ]
}
```

**Note**: The `verbs` list is automatically determined by which interfaces your storage implements.

## Request Flow

```
POST /apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/workspaceconnections
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │ GenericAPIServer              │
                    │ • Authenticates request       │
                    │ • Injects user into context   │
                    │ • Routes to API group         │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │ InstallAPIGroup Router        │
                    │ • Matches route pattern       │
                    │ • Extracts namespace          │
                    │ • Identifies resource         │
                    │ • Calls storage.Create()      │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────────────┐
                    │ WorkspaceConnectionStorage.Create()   │
                    │                                       │
                    │ 1. Decode request body (automatic)    │
                    │ 2. Validate object                    │
                    │ 3. Extract user from context          │
                    │ 4. Extract namespace from context     │
                    │ 5. Check authorization (SAR)          │
                    │ 6. Generate connection URL            │
                    │ 7. Return response object             │
                    └───────────────┬───────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │ GenericAPIServer              │
                    │ • Encodes response (automatic)│
                    │ • Sets proper status code     │
                    │ • Returns to client           │
                    └───────────────────────────────┘
```

## Required Type Definitions

### WorkspaceConnectionList

```go
// +kubebuilder:object:root=true

// WorkspaceConnectionList contains a list of WorkspaceConnection
type WorkspaceConnectionList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []WorkspaceConnectionResponse `json:"items"`
}
```

### Scheme Registration

```go
func AddToScheme(scheme *runtime.Scheme) error {
    scheme.AddKnownTypes(
        schema.GroupVersion{Group: "connection.workspace.jupyter.org", Version: "v1alpha1"},
        &WorkspaceConnectionRequest{},
        &WorkspaceConnectionResponse{},
        &WorkspaceConnectionList{},
    )
    metav1.AddToGroupVersion(scheme, schema.GroupVersion{Group: "connection.workspace.jupyter.org", Version: "v1alpha1"})
    return nil
}
```

## Advantages of InstallAPIGroup

✓ **Automatic routing** - No manual route registration  
✓ **Standard CRUD** - Full REST semantics out of the box  
✓ **Discovery integration** - Automatic `/apis` registration  
✓ **OpenAPI generation** - Schema auto-generated  
✓ **Consistent behavior** - Follows K8s API conventions  
✓ **Client generation** - Works with code generators  
✓ **Validation hooks** - Built-in admission control points  
✓ **Versioning support** - Easy to add v1alpha2, v1beta1, etc.  

## Disadvantages for This Use Case

✗ **Storage requirement** - Must implement storage interface even for ephemeral data  
✗ **CRUD overhead** - Need to handle GET/LIST/UPDATE/DELETE even if not used  
✗ **Complexity** - More boilerplate code  
✗ **Scheme registration** - Additional setup required  
✗ **Codec management** - Serialization/deserialization complexity  
✗ **Not truly stateless** - Framework expects resource persistence model  

## When to Use InstallAPIGroup

Use `InstallAPIGroup` when:

1. **Resources are persistent** - Stored in etcd or external database
2. **Full CRUD needed** - Users need to GET, LIST, UPDATE, DELETE resources
3. **Standard K8s resources** - Following typical resource lifecycle
4. **Multiple versions** - Need to support v1alpha1, v1beta1, v1, etc.
5. **Client generation** - Want to use controller-runtime or client-gen
6. **Watch support** - Need to watch resource changes
7. **Admission webhooks** - Want to integrate with K8s admission control

## When to Use PathRecorderMux (Current Implementation)

Use `PathRecorderMux` when:

1. **RPC-style operations** - Create = execute action, not store resource
2. **Ephemeral responses** - No persistent state
3. **Simple operations** - Only CREATE needed
4. **Custom logic** - Need full control over request/response handling
5. **Stateless** - No storage backend required
6. **Lightweight** - Minimal boilerplate

## Hybrid Approach

You could also combine both:

```go
// Use InstallAPIGroup for persistent resources
apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = map[string]rest.Storage{
    "workspaceconnections": workspaceConnectionStorage,  // InstallAPIGroup
}

// Use PathRecorderMux for RPC-style operations
mux.HandleFunc("/apis/connection.workspace.jupyter.org/v1alpha1/generate-token", 
    handleGenerateToken)  // Direct handler
```

This gives you the best of both worlds:
- Standard CRUD for resources that need it
- Custom handlers for special operations

## File Structure for InstallAPIGroup Implementation

```
internal/extensionapi/
├── server.go                              # Server setup with InstallAPIGroup
├── config.go                              # Configuration management
├── scheme.go                              # Scheme registration
├── storage/
│   ├── workspaceconnection.go            # REST storage implementation
│   ├── workspaceconnection_create.go     # Create operation
│   ├── workspaceconnection_get.go        # Get operation (optional)
│   ├── workspaceconnection_list.go       # List operation (optional)
│   └── storage_test.go                   # Storage tests
├── workspace_admission.go                 # Authorization logic
├── subject_access_review.go               # SAR integration
└── constants.go                           # Constants and headers

api/connection/v1alpha1/
├── workspace_connection_types.go          # Type definitions
├── workspaceconnection_list.go           # List type
├── register.go                            # Scheme registration
└── zz_generated.deepcopy.go              # Generated code
```

## Summary

`InstallAPIGroup` provides a full-featured, resource-oriented API implementation following Kubernetes conventions. However, for the workspace connection use case where:
- Connections are ephemeral (generated on-demand)
- Only CREATE operation is needed
- No persistent storage required
- Simple RPC-style operation

The current `PathRecorderMux` approach is more appropriate and significantly simpler. `InstallAPIGroup` would add unnecessary complexity for managing ephemeral, stateless connection generation.

## References

- [Kubernetes API Server Library](https://github.com/kubernetes/apiserver)
- [Sample API Server](https://github.com/kubernetes/sample-apiserver)
- [API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Building an API Server](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-extension-api-server/)
