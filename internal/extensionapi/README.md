# Extension API Server

A Kubernetes GenericAPIServer-based implementation for generating workspace connections on-demand.

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
│  1. Create JWT Signer Factory (AWS KMS)                             │
│  2. Create SubjectAccessReview Client                               │
│  3. Create GenericAPIServer (K8s apiserver framework)               │
│  4. Create ExtensionServer wrapper                                  │
│  5. Register routes                                                  │
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
│  • PathRecorderMux for routing                                      │
│  • Graceful shutdown                                                │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    ExtensionServer                                   │
│              (internal/extensionapi/server.go)                      │
│                                                                       │
│  Components:                                                         │
│  ├─ k8sClient (controller-runtime client)                          │
│  ├─ sarClient (SubjectAccessReview)                                │
│  ├─ signerFactory (JWT token generation)                           │
│  ├─ logger (request-scoped logging)                                │
│  └─ mux (PathRecorderMux from GenericAPIServer)                    │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │   Route Registration    │
                    └────────────┬────────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        ▼                        ▼                        ▼
┌──────────────┐      ┌──────────────────┐    ┌──────────────────────┐
│   /health    │      │   /apis/.../v1   │    │  /apis/.../v1/       │
│              │      │   (discovery)    │    │  namespaces/{ns}/... │
└──────────────┘      └──────────────────┘    └──────────┬───────────┘
                                                          │
                                         ┌────────────────┴────────────┐
                                         │                             │
                                         ▼                             ▼
                              ┌─────────────────────┐    ┌──────────────────────┐
                              │ workspaceconnections│    │connectionaccessreview│
                              └──────────┬──────────┘    └──────────┬───────────┘
                                         │                           │
                                         ▼                           ▼
                        ┌────────────────────────────┐  ┌────────────────────────┐
                        │ HandleConnectionCreate     │  │handleConnectionAccess  │
                        │                            │  │Review                  │
                        │ 1. Auth check (SAR)       │  │                        │
                        │ 2. Workspace validation   │  │ 1. Check workspace     │
                        │ 3. Generate connection:   │  │    access permissions  │
                        │    • VSCode Remote (SSM)  │  │ 2. Return allowed/     │
                        │    • Web UI (JWT token)   │  │    denied result       │
                        └────────────────────────────┘  └────────────────────────┘
```

## Key Components

### 1. GenericAPIServer Integration

Uses Kubernetes' `genericapiserver` package for production-grade API server capabilities:
- Built-in authentication via RecommendedOptions
- TLS/HTTPS termination
- Graceful shutdown
- Request context management (user info injection)

### 2. ExtensionServer Wrapper

Wraps GenericAPIServer with cuspom business logic:
- Manages route registration and middleware
- Implements `controller-runtime.Runnable` interface
- Provides workspace connection generation

### 3. Middleware Stack

```
Request → Logger Middleware → Route Handler → Response
          (adds logger to context)
```

Every request gets a request-scoped logger with method, path, and remote address.

### 4. Route Architecture

**Simple routes**: Direct path registration
- `/health` - Health check endpoint
- `/apis/connection.workspace.jupyter.org/v1alpha1` - API discovery

**Namespaced routes**: Prefix-based routing with dynamic resource extraction
- Single handler for `/apis/.../namespaces/{namespace}/*`
- Routes to appropriate resource handler based on URL parsing
- Optimized: one prefix handler instead of multiple route registrations

## Why PathRecorderMux Instead of InstallAPIGroup?

### PathRecorderMux (Chosen Approach)

✓ **Direct HTTP handler registration** - Simple function-based routing  
✓ **No storage backend needed** - Stateless, ephemeral responses  
✓ **RPC-style operations** - Create connection = generate URL on-demand  
✓ **Custom request/response handling** - Full control over JSON format  
✓ **Simpler for stateless operations** - No CRUD boilerplate  

### InstallAPIGroup (Not Used)

✗ **Requires REST storage implementation** - Would need to implement full storage interface  
✗ **Expects full CRUD semantics** - Get, List, Create, Update, Delete, Watch  
✗ **Needs scheme/codec registration** - Additional boilerplate  
✗ **Overkill for connection generation** - Too heavy for simple RPC operations  
✗ **Would need to fake storage** - Connections are ephemeral, not persisted  

### The Key Insight

**Connection generation is stateless and ephemeral** - there's nothing to store, list, or update. A workspace connection URL is generated on-demand and returned immediately. This makes PathRecorderMux the perfect fit.

## API Discovery

The server implements standard Kubernetes API discovery:

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
      "verbs": ["create"]
    },
    {
      "name": "connectionaccessreviews",
      "singularName": "connectionaccessreview",
      "namespaced": true,
      "kind": "ConnectionAccessReview",
      "verbs": ["create"]
    }
  ]
}
```

This enables:
- `kubectl api-resources` discovery
- Client library generation
- Standard K8s API conventions
- Documentation of available operations (only "create" supported)

## Request Flow: Creating a Workspace Connection

```
POST /apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/workspaceconnections

Body:
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "WorkspaceConnection",
  "spec": {
    "workspaceName": "my-workspace",
    "workspaceConnectionType": "web-ui"  // or "vscode-remote"
  }
}
```

### Processing Steps

1. **GenericAPIServer**
   - Authenticates request
   - Injects user into context (`request.UserFrom(ctx)`)

2. **loggerMiddleware**
   - Adds request-scoped logger to context
   - Logs method, path, remote address

3. **Namespaced Route Handler**
   - Extracts namespace from path using regex
   - Extracts resource name from URL
   - Routes to specific handler

4. **HandleConnectionCreate**
   - Parses JSON body
   - Validates request (workspaceName, connectionType)
   - Checks authorization using SubjectAccessReview
   - Generates connection URL based on type:
     - **web-ui**: JWT token + bearer auth URL
     - **vscode-remote**: SSM connection URL
   - Returns WorkspaceConnectionResponse

### Response

```json
{
  "apiVersion": "connection.workspace.jupyter.org/v1alpha1",
  "kind": "WorkspaceConnection",
  "spec": {
    "workspaceName": "my-workspace",
    "workspaceConnectionType": "web-ui"
  },
  "status": {
    "workspaceConnectionType": "web-ui",
    "workspaceConnectionUrl": "https://example.com/bearer-auth?token=eyJ..."
  }
}
```

## Authentication & Authorization

### Authentication Flow

```
GenericAPIServer → RecommendedOptions → Authentication
                                       ↓
                           User injected into request.Context
                                       ↓
                           GetUser(r) extracts from context
                                       ↓
                           Used for SAR authorization checks
```

### Authorization Components

**SubjectAccessReview (SAR)**: K8s RBAC integration
- Checks if user can access workspace
- Validates against K8s RBAC policies
- Respects namespace boundaries

**JWT Signing**: AWS KMS-based token generation
- Secure token generation for web UI connections
- Configurable expiration
- Domain and path-specific claims

**Workspace Admission**: Custom authorization logic
- Checks workspace visibility (public/private)
- Validates user permissions
- Returns detailed admission results

## Connection Types

### VSCode Remote

Generates SSM-based connection URLs for remote development:
- Requires `CLUSTER_ID` environment variable
- Uses AWS Systems Manager for secure access
- Retrieves pod UID from workspace
- Generates `vscode-remote://` URL

### Web UI

Generates bearer token URLs with JWT authentication:
- Uses `BearerAuthURLTemplate` from WorkspaceAccessStrategy
- Generates JWT token with user claims
- Includes domain and path in token
- Returns URL with token parameter

## Configuration

### Environment Variables

- `CLUSTER_ID` - Required for VSCode remote connections
- `KMS_KEY_ID` - AWS KMS key for JWT signing
- `DOMAIN` - Domain for Web UI URLs

### Server Options

```go
config := extensionapi.NewConfig(
    extensionapi.WithServerPort(7443),
    extensionapi.WithClusterId(os.Getenv("CLUSTER_ID")),
    extensionapi.WithKMSKeyID(os.Getenv("KMS_KEY_ID")),
    extensionapi.WithDomain(os.Getenv("DOMAIN")),
)
```

### TLS Configuration

- Port: 7443 (separate from metrics/webhooks)
- Certificates: Mounted from volumes
- Default paths:
  - Cert: `/tmp/extension-server/serving-certs/tls.crt`
  - Key: `/tmp/extension-server/serving-certs/tls.key`

## Design Decisions

### 1. Namespaced Routing Optimization

Instead of registering multiple routes:
```go
// ✗ Multiple registrations
mux.HandleFunc("/apis/.../namespaces/*/workspaceconnections", ...)
mux.HandleFunc("/apis/.../namespaces/*/connectionaccessreview", ...)
```

Uses a single prefix handler:
```go
// ✓ Single prefix handler
mux.HandlePrefix("/apis/.../namespaces/", singleHandler)
// Handler dynamically routes to resource handlers
```

Benefits:
- Reduces route registration overhead
- Easier to add new resources
- Centralized namespace extraction

### 2. Manual Discovery Implementation

Manually constructs APIResourceList in `handleDiscovery()`:
- Returns standard K8s format
- Documents available verbs (only "create")
- No GET/LIST/UPDATE/DELETE (not needed for ephemeral resources)
- Enables kubectl discovery

### 3. Middleware Pattern

`loggerMiddleware` wraps all handlers:
- Creates request-scoped logger with context
- Adds to context via `AddLoggerToContext()`
- Retrieved via `GetLoggerFromContext()`
- Enables structured logging per request

### 4. Response Format

- **Errors**: Standard K8s Status format (`WriteKubernetesError`)
- **Success**: Custom types (`WorkspaceConnectionResponse`)
- **Not stored**: Ephemeral, generated on-demand
- **K8s compatible**: Includes TypeMeta + ObjectMeta

## File Structure

```
internal/extensionapi/
├── server.go                              # Main server implementation
├── config.go                              # Configuration management
├── serverroute_discovery.go               # API discovery endpoint
├── serverroute_health.go                  # Health check endpoint
├── connection_route.go                    # Connection creation handler
├── serverroute_connection_access_review.go # Access review handler
├── workspace_admission.go                 # Authorization logic
├── subject_access_review.go               # SAR integration
├── utils.go                               # Helper functions
├── logging.go                             # Context-based logging
└── constants.go                           # Constants and headers
```

## Usage

### Enable in Controller Manager

```bash
./manager --enable-extension-api
```

### Create a Connection

```bash
kubectl create -f - <<EOF
apiVersion: connection.workspace.jupyter.org/v1alpha1
kind: WorkspaceConnection
metadata:
  name: my-connection
  namespace: default
spec:
  workspaceName: my-workspace
  workspaceConnectionType: web-ui
EOF
```

### Check API Resources

```bash
kubectl api-resources --api-group=connection.workspace.jupyter.org
```

## References

- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [GenericAPIServer Documentation](https://pkg.go.dev/k8s.io/apiserver/pkg/server)
- [Building Custom API Servers](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-extension-api-server/)
