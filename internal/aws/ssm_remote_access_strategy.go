package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// Variables for dependency injection in tests
var (
	newSSMClient = NewSSMClient
)

// SSMRemoteAccessClientInterface defines the interface for SSM operations needed by the remote access strategy.
type SSMRemoteAccessClientInterface interface {
	CreateActivation(ctx context.Context, description string, instanceName string, iamRole string, tags map[string]string) (*SSMActivation, error)
	GetRegion() string
	CleanupManagedInstancesByPodUID(ctx context.Context, podUID string) error
	CleanupActivationsByPodUID(ctx context.Context, podUID string) error
	FindInstanceByPodUID(ctx context.Context, podUID string) (string, error)
	StartSession(ctx context.Context, instanceID, documentName, port string) (*SessionInfo, error)
}

// PodExecInterface defines the interface for executing commands in pods.
type PodExecInterface interface {
	ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error)
}

// Constants for SSM remote access strategy
const (
	// Container names
	SSMAgentSidecarContainerName = "ssm-agent-sidecar"
	WorkspaceContainerName       = "workspace"

	// Tag keys
	TagManagedBy       = "managed-by"
	TagWorkspaceName   = "workspace-name"
	TagNamespace       = "namespace"
	TagWorkspacePodUID = "workspace-pod-uid"

	// File paths
	// State file in shared volume (survives container restarts)
	SSMRegistrationStateFile = "/opt/amazon/sagemaker/workspace/.ssm-registration-state.json"

	// TODO: Can be removed once readiness probe is updated to check state file
	SSMRegistrationMarkerFile    = "/tmp/ssm-registered"
	SSMRegistrationScript        = "/usr/local/bin/register-ssm.sh"
	RemoteAccessServerScriptPath = "/opt/amazon/sagemaker/workspace/remote-access/start-remote-access-server.sh"

	// Remote access server configuration
	RemoteAccessServerPort = "2222"
)

// SSMRemoteAccessStrategy handles SSM remote access strategy operations
type SSMRemoteAccessStrategy struct {
	ssmClient   SSMRemoteAccessClientInterface
	podExecUtil PodExecInterface
}

// RegistrationState tracks SSM registration state across container restarts
type RegistrationState struct {
	SidecarRestartCount   int32     `json:"sidecarRestartCount"`
	WorkspaceRestartCount int32     `json:"workspaceRestartCount"`
	SetupInProgress       bool      `json:"setupInProgress,omitempty"`
	SetupStartedAt        time.Time `json:"setupStartedAt,omitempty"`
}

// NewSSMRemoteAccessStrategy creates a new SSMRemoteAccessStrategy with optional dependency injection
// If ssmClient or podExecUtil are nil, default implementations will be created
func NewSSMRemoteAccessStrategy(ssmClient SSMRemoteAccessClientInterface, podExecUtil PodExecInterface) (*SSMRemoteAccessStrategy, error) {
	// If SSM client not provided, create default
	if ssmClient == nil {
		realSSMClient, err := newSSMClient(context.Background())
		if err != nil {
			// Log error but don't fail - SSM features will be disabled to avoid blocking pod operations
			logf.Log.Error(err, "Failed to initialize SSM client")
		}
		ssmClient = realSSMClient
	}

	// PodExecUtil must be provided by the caller since it's in the controller package
	if podExecUtil == nil {
		return nil, fmt.Errorf("podExecUtil is required")
	}

	return &SSMRemoteAccessStrategy{
		ssmClient:   ssmClient,
		podExecUtil: podExecUtil,
	}, nil
}

// SetupContainers initializes SSM agent and remote access server for workspace containers.
// Handles both first-time setup and recovery from container restarts.
//
// Container Restart Detection:
// - Uses persistent state file (/opt/amazon/sagemaker/workspace/.ssm-registration-state.json)
// - Compares stored restart counts with current pod status to detect restarts
// - Selectively re-setups only affected containers (sidecar and/or workspace)
//
// Concurrency Protection:
// - Random delay (0-2s) spreads out concurrent pod events
// - SetupInProgress flag prevents duplicate setup attempts
// - TODO: Consider using distributed mutex for stronger concurrency guarantees
//
// Status Updates:
// - Updates workspace status after creating each SSM resource (activation, managed instance)
// - Requires k8sClient parameter to perform status updates
func (s *SSMRemoteAccessStrategy) SetupContainers(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy, k8sClient client.Client) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspace.Name)

	if s.ssmClient == nil {
		return fmt.Errorf("SSM client not available")
	}

	// Early exit if sidecar not running yet
	if !s.isContainerRunning(pod, SSMAgentSidecarContainerName) {
		logger.V(1).Info("Sidecar container not running yet, waiting")
		return nil
	}

	// Random delay to spread out concurrent events
	delay := time.Duration(rand.Intn(2000)) * time.Millisecond
	logger.V(1).Info("Adding random delay before setup", "delay", delay)
	time.Sleep(delay)

	// Get current restart counts
	currentSidecarRestarts, currentWorkspaceRestarts := s.getCurrentRestartCounts(pod)
	logger.V(1).Info("Current restart counts",
		"sidecar", currentSidecarRestarts,
		"workspace", currentWorkspaceRestarts)

	// Read state from shared volume
	state, err := s.readRegistrationState(ctx, pod)

	// Check if setup is already in progress
	if state != nil && state.SetupInProgress {
		logger.V(1).Info("Setup already in progress by another event, skipping")
		return nil
	}

	// Determine what needs to be setup
	var needSidecarSetup, needWorkspaceSetup, needCleanup bool

	if state == nil {
		if err != nil {
			// File exists but is corrupted - need cleanup
			logger.Info("State file was corrupted, will cleanup before setup")
			needCleanup = true
		} else {
			// File doesn't exist - first time setup, no cleanup needed
			logger.V(1).Info("No state file found, first time setup")
			needCleanup = false
		}
		needSidecarSetup = true
		needWorkspaceSetup = true
	} else {
		// Check for restarts
		sidecarRestarted := currentSidecarRestarts > state.SidecarRestartCount
		workspaceRestarted := currentWorkspaceRestarts > state.WorkspaceRestartCount

		if !sidecarRestarted && !workspaceRestarted {
			logger.V(1).Info("No restarts detected, already setup")
			return nil
		}

		logger.Info("Container restart detected",
			"sidecarRestarted", sidecarRestarted,
			"workspaceRestarted", workspaceRestarted)

		needSidecarSetup = sidecarRestarted
		needWorkspaceSetup = workspaceRestarted
		needCleanup = sidecarRestarted // Only cleanup if sidecar restarted
	}

	// Mark setup as in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   currentSidecarRestarts,
		WorkspaceRestartCount: currentWorkspaceRestarts,
		SetupInProgress:       true,
		SetupStartedAt:        time.Now(),
	}
	if err := s.writeRegistrationState(ctx, pod, inProgressState); err != nil {
		logger.Error(err, "Failed to write in-progress state, continuing anyway")
	} else {
		logger.V(1).Info("Marked setup as in progress")
	}

	// Setup containers as needed
	var setupErr error
	if needSidecarSetup {
		logger.V(1).Info("Setting up sidecar container")
		if err := s.setupSidecarContainer(ctx, pod, workspace, accessStrategy, needCleanup, k8sClient); err != nil {
			setupErr = err
		}
	}

	if setupErr == nil && needWorkspaceSetup {
		logger.V(1).Info("Setting up workspace container")
		if err := s.setupWorkspaceContainer(ctx, pod); err != nil {
			setupErr = err
		}
	}

	// Mark setup as complete (or failed)
	finalState := &RegistrationState{
		SidecarRestartCount:   currentSidecarRestarts,
		WorkspaceRestartCount: currentWorkspaceRestarts,
		SetupInProgress:       false, // Clear the flag
	}
	if err := s.writeRegistrationState(ctx, pod, finalState); err != nil {
		logger.Error(err, "Failed to write final state")
	} else {
		logger.V(1).Info("Marked setup as complete")
	}

	if setupErr != nil {
		return setupErr
	}

	logger.Info("Container setup completed")
	return nil
}

// CleanupSSMManagedNodes finds and deregisters SSM managed nodes for a pod
func (s *SSMRemoteAccessStrategy) CleanupSSMManagedNodes(ctx context.Context, pod *corev1.Pod) error {
	if s.ssmClient == nil {
		return fmt.Errorf("SSM client not available")
	}

	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	// Cleanup managed instances
	if err := s.ssmClient.CleanupManagedInstancesByPodUID(ctx, string(pod.UID)); err != nil {
		logger.Error(err, "Failed to cleanup managed instances")
		// Don't return early - try to cleanup activations too
	}

	// Cleanup hybrid activations
	if err := s.ssmClient.CleanupActivationsByPodUID(ctx, string(pod.UID)); err != nil {
		logger.Error(err, "Failed to cleanup activations")
		return fmt.Errorf("failed to cleanup activations for pod %s: %w", pod.UID, err)
	}

	return nil
}

// isContainerRunning checks if a container is in running state
func (s *SSMRemoteAccessStrategy) isContainerRunning(pod *corev1.Pod, containerName string) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			return cs.State.Running != nil
		}
	}
	return false
}

// getCurrentRestartCounts extracts restart counts from pod status
func (s *SSMRemoteAccessStrategy) getCurrentRestartCounts(pod *corev1.Pod) (sidecarCount, workspaceCount int32) {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == SSMAgentSidecarContainerName {
			sidecarCount = cs.RestartCount
		}
		if cs.Name == WorkspaceContainerName {
			workspaceCount = cs.RestartCount
		}
	}
	return sidecarCount, workspaceCount
}

// readRegistrationState reads state from shared volume
// Returns:
//   - (*RegistrationState, nil) if state file exists and is valid
//   - (nil, nil) if state file doesn't exist (first time setup)
//   - (nil, error) if state file exists but is corrupted
func (s *SSMRemoteAccessStrategy) readRegistrationState(ctx context.Context, pod *corev1.Pod) (*RegistrationState, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	cmd := []string{"cat", SSMRegistrationStateFile}
	output, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, cmd, "")
	if err != nil {
		// File doesn't exist - this is first time setup
		logger.V(1).Info("State file not found, treating as first registration")
		return nil, nil
	}

	var state RegistrationState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		// File exists but is corrupted - need cleanup
		logger.Error(err, "Failed to parse state file, corrupted state detected")
		return nil, err
	}

	logger.V(1).Info("Read registration state",
		"sidecarRestartCount", state.SidecarRestartCount,
		"workspaceRestartCount", state.WorkspaceRestartCount)
	return &state, nil
}

// writeRegistrationState writes state to shared volume
func (s *SSMRemoteAccessStrategy) writeRegistrationState(ctx context.Context, pod *corev1.Pod, state *RegistrationState) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file then move (atomic operation)
	cmd := []string{"bash", "-c", fmt.Sprintf("echo '%s' > %s.tmp && mv %s.tmp %s",
		string(stateJSON), SSMRegistrationStateFile, SSMRegistrationStateFile, SSMRegistrationStateFile)}

	if _, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, cmd, ""); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	logger.V(1).Info("Wrote registration state",
		"sidecarRestartCount", state.SidecarRestartCount,
		"workspaceRestartCount", state.WorkspaceRestartCount)
	return nil
}

// setupSidecarContainer handles SSM agent registration and updates workspace status
func (s *SSMRemoteAccessStrategy) setupSidecarContainer(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy, cleanupNeeded bool, k8sClient client.Client) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspace.Name)
	noStdin := ""

	// Cleanup old resources if needed
	if cleanupNeeded {
		logger.V(1).Info("Cleaning up old SSM resources before registration")
		if err := s.CleanupSSMManagedNodes(ctx, pod); err != nil {
			logger.Error(err, "Failed to cleanup old SSM resources, continuing with registration anyway")
		}
	}

	// Create SSM activation
	logger.V(1).Info("Creating SSM activation")
	activationCode, activationId, err := s.createSSMActivation(ctx, pod, workspace, accessStrategy)
	if err != nil {
		return fmt.Errorf("failed to create SSM activation: %w", err)
	}
	logger.Info("SSM activation created", "activationId", activationId)

	// Update status with activation immediately
	region := s.ssmClient.GetRegion()
	podUID := string(pod.UID)
	if err := s.addResourceToStatus(ctx, workspace, k8sClient, workspacev1alpha1.ExternalAccessResourceStatus{
		ResourceType: "SSMActivation",
		Provider:     "aws-ssm",
		ResourceID:   activationId,
		Metadata: map[string]string{
			"podUid":    podUID,
			"podName":   pod.Name,
			"region":    region,
			"createdAt": time.Now().Format(time.RFC3339),
		},
	}); err != nil {
		logger.Error(err, "Failed to update status with activation, continuing anyway")
	}

	// Run registration script
	logger.V(1).Info("Running SSM registration script in sidecar")
	// Use stdin to pass only sensitive values securely
	cmd := []string{"bash", "-c", fmt.Sprintf("read ACTIVATION_ID && read ACTIVATION_CODE && env ACTIVATION_ID=\"$ACTIVATION_ID\" ACTIVATION_CODE=\"$ACTIVATION_CODE\" REGION=%s %s", region, SSMRegistrationScript)}
	stdinData := fmt.Sprintf("%s\n%s\n", activationId, activationCode)

	output, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, cmd, stdinData)
	if err != nil {
		return fmt.Errorf("failed to execute SSM registration script: %w", err)
	}
	logger.Info("SSM registration completed successfully")

	// Extract managed instance ID from output
	managedInstanceID := s.extractManagedInstanceID(output)
	if managedInstanceID == "" {
		logger.Error(nil, "Failed to extract managed instance ID from registration output")
		return fmt.Errorf("failed to extract managed instance ID")
	}
	logger.Info("Extracted managed instance ID", "instanceId", managedInstanceID)

	// Update status with managed instance immediately
	if err := s.addResourceToStatus(ctx, workspace, k8sClient, workspacev1alpha1.ExternalAccessResourceStatus{
		ResourceType: "SSMManagedInstance",
		Provider:     "aws-ssm",
		ResourceID:   managedInstanceID,
		Metadata: map[string]string{
			"podUid":    podUID,
			"podName":   pod.Name,
			"region":    region,
			"createdAt": time.Now().Format(time.RFC3339),
		},
	}); err != nil {
		logger.Error(err, "Failed to update status with managed instance, continuing anyway")
	}

	// TODO: Remove this once all deployments migrate to state-file-based readiness checks
	logger.V(1).Info("Creating marker file for backward compatibility")
	markerCmd := []string{"touch", SSMRegistrationMarkerFile}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, markerCmd, noStdin); err != nil {
		return fmt.Errorf("failed to create marker file: %w", err)
	}

	logger.Info("Sidecar container setup completed")
	return nil
}

// setupWorkspaceContainer starts the remote access server
func (s *SSMRemoteAccessStrategy) setupWorkspaceContainer(ctx context.Context, pod *corev1.Pod) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	// Check workspace is running
	if !s.isContainerRunning(pod, WorkspaceContainerName) {
		logger.V(1).Info("Workspace container not running yet, waiting")
		return nil
	}

	// TODO: Make this script idempotent
	// For now, only called on container restart so should be fine
	// Step 3: Start remote access server in main container using the startup script
	logger.Info("Starting remote access server in main container")
	serverCmd := []string{RemoteAccessServerScriptPath, "--port", RemoteAccessServerPort}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, WorkspaceContainerName, serverCmd, ""); err != nil {
		return fmt.Errorf("failed to start remote access server: %w", err)
	}

	logger.Info("Workspace container setup completed")
	return nil
}

// createSSMActivation creates an SSM activation and returns activation code and ID
func (s *SSMRemoteAccessStrategy) createSSMActivation(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, string, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	if s.ssmClient == nil {
		return "", "", fmt.Errorf("SSM client not available")
	}

	// Get IAM role from access strategy controller configuration
	var iamRole string
	if accessStrategy.Spec.ControllerConfig != nil {
		iamRole = accessStrategy.Spec.ControllerConfig["SSM_MANAGED_NODE_ROLE"]
	}

	if iamRole == "" {
		return "", "", fmt.Errorf("SSM_MANAGED_NODE_ROLE not found in access strategy")
	}

	// Get EKS cluster ARN from environment variable
	eksClusterARN := os.Getenv(EKSClusterARNEnv)
	if eksClusterARN == "" {
		return "", "", fmt.Errorf("%s environment variable is required", EKSClusterARNEnv)
	}

	// Prepare tags - include SageMaker required tags for policy compliance
	tags := map[string]string{
		SageMakerManagedByTagKey:  SageMakerManagedByTagValue,
		SageMakerEKSClusterTagKey: eksClusterARN,
		TagWorkspaceName:          workspace.Name,
		TagNamespace:              workspace.Namespace,
		TagWorkspacePodUID:        string(pod.UID),
	}

	// Create description and instance name with fixed prefix
	description := fmt.Sprintf("Activation for %s/%s (pod: %s)", workspace.Namespace, workspace.Name, string(pod.UID))
	instanceName := fmt.Sprintf("%s-%s", SSMInstanceNamePrefix, string(pod.UID))

	// Pass the IAM role directly to the SSM client
	activation, err := s.ssmClient.CreateActivation(ctx, description, instanceName, iamRole, tags)
	if err != nil {
		logger.Error(err, "Failed to create SSM activation")
		return "", "", fmt.Errorf("failed to create SSM activation: %w", err)
	}

	logger.Info("SSM activation created successfully",
		"activationId", activation.ActivationId,
		"iamRole", iamRole)

	return activation.ActivationCode, activation.ActivationId, nil
}

// GenerateVSCodeConnectionURL generates a VSCode connection URL using SSM session
func (s *SSMRemoteAccessStrategy) GenerateVSCodeConnectionURL(ctx context.Context, workspaceName string, namespace string, podUID string, eksClusterARN string, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, error) {
	logger := logf.FromContext(ctx).WithName("ssm-vscode-connection")

	// Find managed instance by pod UID
	instanceID, err := s.ssmClient.FindInstanceByPodUID(ctx, podUID)
	if err != nil {
		return "", fmt.Errorf("failed to find instance by pod UID: %w", err)
	}

	logger.Info("Found managed instance for pod", "podUID", podUID, "instanceID", instanceID)

	// Get SSM document name from access strategy controller configuration
	var documentName string
	if accessStrategy.Spec.ControllerConfig != nil {
		documentName = accessStrategy.Spec.ControllerConfig["SSM_DOCUMENT_NAME"]
	}

	if documentName == "" {
		return "", fmt.Errorf("SSM_DOCUMENT_NAME not found in access strategy")
	}

	// Start SSM session
	sessionInfo, err := s.ssmClient.StartSession(ctx, instanceID, documentName, RemoteAccessServerPort)
	if err != nil {
		return "", fmt.Errorf("failed to start SSM session: %w", err)
	}

	logger.Info("SSM session started successfully", "instanceID", instanceID, "sessionID", sessionInfo.SessionID)

	// Generate VSCode URL
	url := fmt.Sprintf("%s?sessionId=%s&sessionToken=%s&streamUrl=%s&workspaceName=%s&namespace=%s&eksClusterArn=%s",
		VSCodeScheme,
		sessionInfo.SessionID,
		sessionInfo.TokenValue,
		sessionInfo.StreamURL,
		workspaceName,
		namespace,
		eksClusterARN)

	return url, nil
}

// extractManagedInstanceID extracts managed instance ID from registration script output
func (s *SSMRemoteAccessStrategy) extractManagedInstanceID(output string) string {
	// Look for pattern: "Registration successful - Instance ID: mi-xxxxx"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Registration successful - Instance ID:") {
			parts := strings.Split(line, "Instance ID:")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// addResourceToStatus adds or updates an external resource in workspace status
func (s *SSMRemoteAccessStrategy) addResourceToStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	k8sClient client.Client,
	newResource workspacev1alpha1.ExternalAccessResourceStatus,
) error {
	logger := logf.FromContext(ctx)
	podUID := newResource.Metadata["podUid"]

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch workspace to get latest version
		current := &workspacev1alpha1.Workspace{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      workspace.Name,
			Namespace: workspace.Namespace,
		}, current); err != nil {
			return err
		}

		// Check if resource already exists
		found := false
		for i, resource := range current.Status.ExternalAccessResources {
			if resource.Provider == "aws-ssm" &&
				resource.ResourceType == newResource.ResourceType &&
				resource.Metadata != nil &&
				resource.Metadata["podUid"] == podUID {
				// Update existing
				current.Status.ExternalAccessResources[i] = newResource
				found = true
				logger.V(1).Info("Updated existing external resource in status",
					"resourceType", newResource.ResourceType,
					"resourceId", newResource.ResourceID)
				break
			}
		}

		if !found {
			// Add new
			current.Status.ExternalAccessResources = append(current.Status.ExternalAccessResources, newResource)
			logger.V(1).Info("Added new external resource to status",
				"resourceType", newResource.ResourceType,
				"resourceId", newResource.ResourceID)
		}

		return k8sClient.Status().Update(ctx, current)
	})
}
