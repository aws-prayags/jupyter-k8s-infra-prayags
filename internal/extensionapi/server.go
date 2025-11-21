// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/mux"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/registry/rest"
	openapicommon "k8s.io/kube-openapi/pkg/common"
	spec "k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/apiserver/pkg/util/compatibility"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	setupLog = log.Log.WithName("extension-api-server")
	scheme   = runtime.NewScheme()
	codecs   = serializer.NewCodecFactory(scheme)
)

func init() {
	// Register standard Kubernetes types (required by GenericAPIServer)
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
	
	// Register resource types with the scheme
	scheme.AddKnownTypes(
		schema.GroupVersion{Group: "connection.workspace.jupyter.org", Version: "v1alpha1"},
		&DummyResource{},
		&DummyResourceList{},
		&connectionv1alpha1.WorkspaceConnection{},
		&connectionv1alpha1.WorkspaceConnectionList{},
		&connectionv1alpha1.WorkspaceConnectionRequest{},
		&connectionv1alpha1.WorkspaceConnectionResponse{},
	)
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Group: "connection.workspace.jupyter.org", Version: "v1alpha1"})
}

// ExtensionServer represents the extension API HTTP server
type ExtensionServer struct {
	config        *ExtensionConfig
	k8sClient     client.Client
	sarClient     v1.SubjectAccessReviewInterface
	signerFactory jwt.SignerFactory
	logger        *logr.Logger
	genericServer *genericapiserver.GenericAPIServer
	routes        map[string]func(http.ResponseWriter, *http.Request)
	mux           *mux.PathRecorderMux
}

// NewExtensionServer creates a new extension API server using GenericAPIServer
func NewExtensionServer(
	genericServer *genericapiserver.GenericAPIServer,
	config *ExtensionConfig,
	logger *logr.Logger,
	k8sClient client.Client,
	sarClient v1.SubjectAccessReviewInterface,
	signerFactory jwt.SignerFactory) *ExtensionServer {

	server := &ExtensionServer{
		config:        config,
		logger:        logger,
		k8sClient:     k8sClient,
		sarClient:     sarClient,
		signerFactory: signerFactory,
		routes:        make(map[string]func(http.ResponseWriter, *http.Request)),
		genericServer: genericServer,
		mux:           genericServer.Handler.NonGoRestfulMux,
	}

	return server
}

// loggerMiddleware wraps an http.Handler and adds a logger to the request context
func (s *ExtensionServer) loggerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Create request-specific logger with path info
		reqLogger := (*s.logger).WithValues(
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)

		// Create new context with logger
		ctx := AddLoggerToContext(r.Context(), reqLogger)

		// Call next handler with the augmented context
		next(w, r.WithContext(ctx))
	}
}

// registerRoute registers a route handler
func (s *ExtensionServer) registerRoute(name string, handler func(http.ResponseWriter, *http.Request)) {
	// Store original handler in routes map
	s.routes[name] = handler

	// Ensure the path starts with a slash
	path := name
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Wrap handler with the logger middleware
	wrappedHandler := s.loggerMiddleware(handler)

	// Register the wrapped handler
	s.mux.HandleFunc(path, wrappedHandler)
}

// registerNamespacedRoutes registers multiple route handlers for resources with namespaces in the URL path
// It efficiently handles paths like "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/{resource}"
// by registering a single handler for the namespaced path prefix and routing to the appropriate handler
func (s *ExtensionServer) registerNamespacedRoutes(resourceHandlers map[string]func(http.ResponseWriter, *http.Request)) {
	// Store all the resource handlers in the routes map for reference
	basePattern := s.config.ApiPath
	namespacedPathPrefix := basePattern + "/namespaces/"

	// For each resource, store it in the routes map with a descriptive pattern
	for resource, handler := range resourceHandlers {
		pattern := namespacedPathPrefix + "*/" + resource
		s.routes[pattern] = handler
		setupLog.Info("Added namespaced route", "resource", resource, "pattern", pattern)
	}

	// Create a single wrapped handler that will route to the appropriate resource handler
	wrappedHandler := s.loggerMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Extract namespace from path
		namespace, err := GetNamespaceFromPath(r.URL.Path)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid or missing namespace in path")
			return
		}

		// Extract the resource name from the path
		// The path format is /apis/group/version/namespaces/namespace/resource
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 6 {
			http.NotFound(w, r)
			return
		}

		// The resource should be the last part of the path
		resource := parts[len(parts)-1]

		// Find the handler for this resource
		if handler, ok := resourceHandlers[resource]; ok {
			setupLog.Info("Handling namespaced request",
				"path", r.URL.Path,
				"namespace", namespace,
				"resource", resource)
			handler(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	// Register the single wrapped handler for the namespaced path prefix
	s.mux.HandlePrefix(namespacedPathPrefix, wrappedHandler)
	setupLog.Info("Registered namespaced routes handler", "pathPrefix", namespacedPathPrefix)
}

// registerAllRoutes register the actual routes to the server
func (s *ExtensionServer) registerAllRoutes() {
	// Register health check route
	s.registerRoute("/health", s.handleHealth)

	// Register API discovery route
	s.registerRoute(s.config.ApiPath, s.handleDiscovery)

	// Register all namespaced routes
	s.registerNamespacedRoutes(map[string]func(http.ResponseWriter, *http.Request){
		"workspaceconnections":   s.HandleConnectionCreate,
		"connectionaccessreview": s.handleConnectionAccessReview,
	})
}

// Start starts the extension API server and implements the controller-runtime's Runnable interface
func (s *ExtensionServer) Start(ctx context.Context) error {
	setupLog.Info("Starting extension API server with GenericAPIServer")

	// Prepare and run the GenericAPIServer
	preparedServer := s.genericServer.PrepareRun()
	return preparedServer.RunWithContext(ctx)
}

// NeedLeaderElection implements the LeaderElectionRunnable interface
// This indicates this runnable doesn't need to be a leader to run
func (s *ExtensionServer) NeedLeaderElection() bool {
	return false
}

// createSARClient creates a SubjectAccessReview client from manager
func createSARClient(mgr ctrl.Manager) (v1.SubjectAccessReviewInterface, error) {
	k8sClientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate the sar client: %w", err)
	}
	return k8sClientset.AuthorizationV1().SubjectAccessReviews(), nil
}

// createRecommendedOptions creates GenericAPIServer options from config
func createRecommendedOptions(config *ExtensionConfig) *genericoptions.RecommendedOptions {
	recommendedOptions := genericoptions.NewRecommendedOptions(
		"/unused",
		nil, // No codec needed for our simple case
	)

	// Configure port and certificates
	recommendedOptions.SecureServing.BindPort = config.ServerPort
	recommendedOptions.SecureServing.ServerCert.CertDirectory = ""
	recommendedOptions.SecureServing.ServerCert.CertKey.CertFile = config.CertPath
	recommendedOptions.SecureServing.ServerCert.CertKey.KeyFile = config.KeyPath
	recommendedOptions.SecureServing.ServerCert.PairName = "tls"

	return recommendedOptions
}

// createGenericAPIServer creates a GenericAPIServer from options
func createGenericAPIServer(recommendedOptions *genericoptions.RecommendedOptions, server *ExtensionServer) (*genericapiserver.GenericAPIServer, error) {
	// Create server config
	serverConfig := genericapiserver.NewRecommendedConfig(codecs)
	serverConfig.EffectiveVersion = compatibility.DefaultBuildEffectiveVersion()

	// Apply options to configure authentication automatically
	if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
		return nil, fmt.Errorf("failed to apply recommended options: %w", err)
	}

	// Configure OpenAPI - required for InstallAPIGroup
	// Provide OpenAPI definitions for our custom types
	serverConfig.OpenAPIV3Config = &openapicommon.OpenAPIV3Config{
		Info: &spec.Info{
			InfoProps: spec.InfoProps{
				Title:   "Extension API Server",
				Version: "v1alpha1",
			},
		},
		GetDefinitions: getDummyResourceOpenAPIDefinitions,
	}

	// Create GenericAPIServer
	genericServer, err := serverConfig.Complete().New("extension-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, fmt.Errorf("failed to create generic API server: %w", err)
	}

	// Install dummy API group to test GenericAPIServer integration
	if err := installDummyAPIGroup(genericServer, server); err != nil {
		return nil, fmt.Errorf("failed to install dummy API group: %w", err)
	}

	return genericServer, nil
}

// installDummyAPIGroup installs a minimal dummy resource for testing
func installDummyAPIGroup(genericServer *genericapiserver.GenericAPIServer, server *ExtensionServer) error {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
		"connection.workspace.jupyter.org",
		scheme,
		runtime.NewParameterCodec(scheme),
		codecs,
	)

	// Set version priority
	apiGroupInfo.PrioritizedVersions = []schema.GroupVersion{
		{Group: "connection.workspace.jupyter.org", Version: "v1alpha1"},
	}

	// Register resource storage
	v1alpha1Storage := map[string]rest.Storage{}
	v1alpha1Storage["dummyresources"] = &DummyStorage{}
	v1alpha1Storage["workspaceconnections"] = &WorkspaceConnectionStorage{server: server}
	apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1Storage

	// Install the API group
	setupLog.Info("Installing dummy API group", "group", "connection.workspace.jupyter.org", "version", "v1alpha1")
	if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		setupLog.Error(err, "Failed to install dummy API group")
		return fmt.Errorf("failed to install API group: %w", err)
	}

	setupLog.Info("Successfully installed dummy API group with dummyresources", "group", "connection.workspace.jupyter.org", "version", "v1alpha1")
	return nil
}

// getDummyResourceOpenAPIDefinitions provides OpenAPI definitions for resources
func getDummyResourceOpenAPIDefinitions(ref openapicommon.ReferenceCallback) map[string]openapicommon.OpenAPIDefinition {
	return map[string]openapicommon.OpenAPIDefinition{
		"github.com/jupyter-ai-contrib/jupyter-k8s/internal/extensionapi.DummyResource": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"apiVersion": {
							SchemaProps: spec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"kind": {
							SchemaProps: spec.SchemaProps{
								Type:   []string{"string"},
								Format: "",
							},
						},
						"metadata": {
							SchemaProps: spec.SchemaProps{
								Type:   []string{"object"},
								Format: "",
							},
						},
					},
				},
			},
		},
		"github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1.WorkspaceConnection": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"apiVersion": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"kind": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"metadata": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"object"},
							},
						},
						"spec": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"object"},
								Properties: map[string]spec.Schema{
									"workspaceName": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
									"workspaceConnectionType": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1.WorkspaceConnectionRequest": {
			Schema: spec.Schema{
				SchemaProps: spec.SchemaProps{
					Type: []string{"object"},
					Properties: map[string]spec.Schema{
						"apiVersion": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"kind": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"string"},
							},
						},
						"metadata": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"object"},
							},
						},
						"spec": {
							SchemaProps: spec.SchemaProps{
								Type: []string{"object"},
								Properties: map[string]spec.Schema{
									"workspaceName": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
									"workspaceConnectionType": {
										SchemaProps: spec.SchemaProps{
											Type: []string{"string"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createJWTSignerFactory(config *ExtensionConfig) (jwt.SignerFactory, error) {
	// Create KMS client and signer factory
	ctx := context.Background()
	kmsClient, err := aws.NewKMSClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create KMS client: %w", err)
	}

	signerFactory := aws.NewAWSSignerFactory(kmsClient, config.KMSKeyID, time.Minute*5)

	return signerFactory, nil
}

// createExtensionServer creates and configures the extension server
func createExtensionServer(genericServer *genericapiserver.GenericAPIServer, config *ExtensionConfig, logger *logr.Logger, k8sClient client.Client, sarClient v1.SubjectAccessReviewInterface, jwtSingerFactory jwt.SignerFactory) *ExtensionServer {
	server := NewExtensionServer(genericServer, config, logger, k8sClient, sarClient, jwtSingerFactory)
	server.registerAllRoutes()
	return server
}

// addServerToManager adds the server to the controller manager
func addServerToManager(mgr ctrl.Manager, server *ExtensionServer) error {
	if err := mgr.Add(server); err != nil {
		return fmt.Errorf("failed to add extension API server to manager: %w", err)
	}
	return nil
}

// SetupExtensionAPIServerWithManager sets up the extension API server and adds it to the manager
func SetupExtensionAPIServerWithManager(mgr ctrl.Manager, config *ExtensionConfig) error {
	// Use the config or create a default config
	if config == nil {
		config = NewConfig()
	}

	logger := mgr.GetLogger().WithName("extension-api")

	// Create JWT manager
	signerFactory, err := createJWTSignerFactory(config)
	if err != nil {
		return err
	}

	// Create SAR client
	sarClient, err := createSARClient(mgr)
	if err != nil {
		return err
	}

	// Create a placeholder server first (without genericServer)
	server := &ExtensionServer{
		config:        config,
		logger:        &logger,
		k8sClient:     mgr.GetClient(),
		sarClient:     sarClient,
		signerFactory: signerFactory,
		routes:        make(map[string]func(http.ResponseWriter, *http.Request)),
	}

	// Create GenericAPIServer with the server reference
	recommendedOptions := createRecommendedOptions(config)
	genericServer, err := createGenericAPIServer(recommendedOptions, server)
	if err != nil {
		return err
	}

	// Complete the server setup
	server.genericServer = genericServer
	server.mux = genericServer.Handler.NonGoRestfulMux
	server.registerAllRoutes()

	// Add server to manager
	return addServerToManager(mgr, server)
}
