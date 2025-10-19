package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// IdleResponse represents the response from /api/idle endpoint
type IdleResponse struct {
	LastActivity string `json:"last_activity"`
}

// WorkspaceIdleChecker provides utilities for checking workspace idle status
type WorkspaceIdleChecker struct {
	client    client.Client
	clientset *kubernetes.Clientset
	config    *rest.Config
}

// NewWorkspaceIdleChecker creates a new WorkspaceIdleChecker instance
func NewWorkspaceIdleChecker(client client.Client) *WorkspaceIdleChecker {
	cfg, err := config.GetConfig()
	if err != nil {
		return &WorkspaceIdleChecker{
			client: client,
		} // Fallback to mock behavior
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return &WorkspaceIdleChecker{
			client: client,
		} // Fallback to mock behavior
	}

	return &WorkspaceIdleChecker{
		client:    client,
		clientset: clientset,
		config:    cfg,
	}
}

// CheckWorkspaceIdle calls the /api/idle endpoint using kubectl exec equivalent
func (w *WorkspaceIdleChecker) CheckWorkspaceIdle(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (*IdleResponse, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name, "namespace", workspace.Namespace)

	// Find the workspace pod
	pod, err := w.findWorkspacePod(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to find workspace pod")
		return nil, fmt.Errorf("failed to find workspace pod: %w", err)
	}

	logger.V(1).Info("Found workspace pod", "pod", pod.Name)

	// Fallback to mock behavior if clientset not available
	if w.clientset == nil || w.config == nil {
		logger.Info("Would check idle status via exec (mock)")
		return &IdleResponse{LastActivity: time.Now().Format(time.RFC3339)}, nil
	}

	// Try curl first, then fallback to python if curl fails
	output, err := w.tryIdleEndpointCall(ctx, pod)
	if err != nil {
		logger.Error(err, "Failed to check idle endpoint")
		return nil, fmt.Errorf("failed to check idle status: %w", err)
	}

	// Parse the JSON response
	var idleResp IdleResponse
	if err := json.Unmarshal([]byte(output), &idleResp); err != nil {
		logger.Error(err, "Failed to parse idle response", "output", output)
		return nil, fmt.Errorf("failed to parse idle response: %w", err)
	}

	// Validate the response
	if idleResp.LastActivity == "" {
		logger.Error(nil, "Empty last_activity in response", "output", output)
		return nil, fmt.Errorf("invalid idle response: empty last_activity")
	}

	logger.V(1).Info("Successfully retrieved idle status", "lastActivity", idleResp.LastActivity)
	return &idleResp, nil
}

// findWorkspacePod finds the pod for a workspace
func (w *WorkspaceIdleChecker) findWorkspacePod(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (*corev1.Pod, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// List pods with the workspace labels
	podList := &corev1.PodList{}
	labels := GenerateLabels(workspace.Name)

	if err := w.client.List(ctx, podList, client.InNamespace(workspace.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Find a running pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			logger.V(1).Info("Found running workspace pod", "pod", pod.Name)
			return &pod, nil
		}
	}

	return nil, fmt.Errorf("no running pod found for workspace")
}

// execInPod executes a command in a pod (similar to kubectl exec)
func (w *WorkspaceIdleChecker) execInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string) (string, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "container", containerName, "cmd", cmd)

	// Create exec request
	req := w.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	// Execute command
	exec, err := remotecommand.NewSPDYExecutor(w.config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	output := strings.TrimSpace(stdout.String())
	if err != nil {
		logger.V(1).Info("Command execution failed", "error", err, "stderr", stderr.String())
		return output, err
	}

	logger.V(1).Info("Command executed successfully", "output", output)
	return output, nil
}

// tryIdleEndpointCall tries multiple methods to call the idle endpoint
func (w *WorkspaceIdleChecker) tryIdleEndpointCall(ctx context.Context, pod *corev1.Pod) (string, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	// Try curl first
	curlCmd := []string{"curl", "-s", "http://localhost:8888/api/idle"}
	logger.V(1).Info("Trying curl", "cmd", curlCmd)
	
	output, err := w.execInPod(ctx, pod, "", curlCmd)
	if err == nil && output != "" {
		logger.V(1).Info("curl succeeded")
		return output, nil
	}
	logger.V(1).Info("curl failed, trying python", "error", err)

	// Fallback to python
	pythonCmd := []string{"python3", "-c", `
import urllib.request
import json
try:
    with urllib.request.urlopen('http://localhost:8888/api/idle') as response:
        data = json.loads(response.read().decode())
        print(json.dumps(data))
except Exception as e:
    print(f'{{"error": "{e}"}}')
`}
	
	logger.V(1).Info("Trying python", "cmd", pythonCmd)
	output, err = w.execInPod(ctx, pod, "", pythonCmd)
	if err == nil && output != "" {
		logger.V(1).Info("python succeeded")
		return output, nil
	}
	logger.V(1).Info("python failed, trying wget", "error", err)

	// Fallback to wget
	wgetCmd := []string{"wget", "-qO-", "http://localhost:8888/api/idle"}
	logger.V(1).Info("Trying wget", "cmd", wgetCmd)
	
	output, err = w.execInPod(ctx, pod, "", wgetCmd)
	if err == nil && output != "" {
		logger.V(1).Info("wget succeeded")
		return output, nil
	}

	return "", fmt.Errorf("all methods failed - curl: %v, python: available but failed, wget: %v", err, err)
}