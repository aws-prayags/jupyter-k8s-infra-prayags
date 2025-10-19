package controller

import (
	"context"
	"fmt"
	"time"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StateMachine handles the state transitions for Workspace
type StateMachine struct {
	resourceManager  *ResourceManager
	statusManager    *StatusManager
	templateResolver *TemplateResolver
	recorder         record.EventRecorder
	idleChecker      *WorkspaceIdleChecker
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(resourceManager *ResourceManager, statusManager *StatusManager, templateResolver *TemplateResolver, recorder record.EventRecorder, idleChecker *WorkspaceIdleChecker) *StateMachine {
	return &StateMachine{
		resourceManager:  resourceManager,
		statusManager:    statusManager,
		templateResolver: templateResolver,
		recorder:         recorder,
		idleChecker:      idleChecker,
	}
}

// ReconcileDesiredState handles the state machine logic for Workspace
func (sm *StateMachine) ReconcileDesiredState(
	ctx context.Context, workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(workspace)
	snapshotStatus := workspace.DeepCopy().Status

	switch desiredStatus {
	case "Stopped":
		return sm.reconcileDesiredStoppedStatus(ctx, workspace, &snapshotStatus)
	case "Running":
		return sm.reconcileDesiredRunningStatus(ctx, workspace, &snapshotStatus)
	default:
		err := fmt.Errorf("unknown desired status: %s", desiredStatus)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), &snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, err
	}
}

// getDesiredStatus returns the desired status with default fallback
func (sm *StateMachine) getDesiredStatus(workspace *workspacesv1alpha1.Workspace) string {
	if workspace.Spec.DesiredStatus == "" {
		return DefaultDesiredStatus
	}
	return workspace.Spec.DesiredStatus
}

func (sm *StateMachine) reconcileDesiredStoppedStatus(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	snapshotStatus *workspacesv1alpha1.WorkspaceStatus) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring Workspace status to 'Stopped'")

	// Remove access strategy resources first
	accessError := sm.ReconcileAccessForDesiredStoppedStatus(ctx, workspace)
	if accessError != nil {
		logger.Error(accessError, "Failed to remove access strategy resources")
		// Continue with deletion of other resources, don't block on access strategy
	}

	// Ensure deployment is deleted - this is an asynchronous operation
	// EnsureDeploymentDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the deployment to be fully removed
	deployment, deploymentErr := sm.resourceManager.EnsureDeploymentDeleted(ctx, workspace)
	if deploymentErr != nil {
		err := fmt.Errorf("failed to get deployment: %w", deploymentErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Ensure service is deleted - this is an asynchronous operation
	// EnsureServiceDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the service to be fully removed
	service, serviceErr := sm.resourceManager.EnsureServiceDeleted(ctx, workspace)
	if serviceErr != nil {
		err := fmt.Errorf("failed to get service: %w", serviceErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonServiceError, err.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Check if resources are fully deleted (asynchronous deletion check)
	// A nil resource means the resource has been fully deleted
	deploymentDeleted := sm.resourceManager.IsDeploymentMissingOrDeleting(deployment)
	serviceDeleted := sm.resourceManager.IsServiceMissingOrDeleting(service)
	accessResourcesDeleted := sm.resourceManager.AreAccessResourcesDeleted(workspace)

	if deploymentDeleted && serviceDeleted {
		// Flag as Error if AccessResources failed to delete
		if accessError != nil {
			if statusErr := sm.statusManager.UpdateErrorStatus(
				ctx, workspace, ReasonServiceError, accessError.Error(), snapshotStatus); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, accessError
		} else if !accessResourcesDeleted {
			// AccessResources are not fully deleted, requeue
			readiness := WorkspaceStoppingReadiness{
				computeStopped:         deploymentDeleted,
				serviceStopped:         serviceDeleted,
				accessResourcesStopped: accessResourcesDeleted,
			}
			if err := sm.statusManager.UpdateStoppingStatus(ctx, workspace, readiness, snapshotStatus); err != nil {
				return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
		} else {
			// All resources are fully deleted, update to stopped status
			logger.Info("Deployment and Service are both deleted, updating to Stopped status")

			// Record workspace stopped event
			sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceStopped", "Workspace has been stopped")

			if err := sm.statusManager.UpdateStoppedStatus(ctx, workspace, snapshotStatus); err != nil {
				return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// If EITHER deployment OR service is still in the process of being deleted
	// Update status to Stopping and requeue to check again later
	if deploymentDeleted || serviceDeleted {
		logger.Info("Resources still being deleted", "deploymentDeleted", deploymentDeleted, "serviceDeleted", serviceDeleted)
		readiness := WorkspaceStoppingReadiness{
			computeStopped:         deploymentDeleted,
			serviceStopped:         serviceDeleted,
			accessResourcesStopped: accessResourcesDeleted,
		}
		if err := sm.statusManager.UpdateStoppingStatus(
			ctx, workspace, readiness, snapshotStatus); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		// Requeue to check deletion progress again later
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// This should not happen, return an error
	err := fmt.Errorf("unexpected state: both deployment and service should be in deletion process")
	// Update error condition
	if statusErr := sm.statusManager.UpdateErrorStatus(
		ctx, workspace, ReasonDeploymentError, err.Error(), snapshotStatus); statusErr != nil {
		logger.Error(statusErr, "Failed to update error status")
	}
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
}

func (sm *StateMachine) reconcileDesiredRunningStatus(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	snapshotStatus *workspacesv1alpha1.WorkspaceStatus) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring Workspace status to 'Running'")

	// Validate template BEFORE creating any resources
	resolvedTemplate, shouldContinue, err := sm.handleTemplateValidation(ctx, workspace, snapshotStatus)
	if !shouldContinue {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Ensure PVC exists first (if storage is configured)
	_, err = sm.resourceManager.EnsurePVCExists(ctx, workspace, resolvedTemplate)
	if err != nil {
		pvcErr := fmt.Errorf("failed to ensure PVC exists: %w", err)
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, pvcErr.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, pvcErr
	}

	// Ensure deployment exists (pass the resolved template)
	// EnsureDeploymentExists internally fetches the deployment and returns it with current status
	// On first reconciliation: creates deployment (status will be empty initially)
	// On subsequent reconciliations: returns existing deployment with populated status
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace, resolvedTemplate)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, deployErr.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, deployErr
	}

	// Ensure service exists
	// EnsureServiceExists internally fetches the service and returns it with current status
	service, err := sm.resourceManager.EnsureServiceExists(ctx, workspace)
	if err != nil {
		serviceErr := fmt.Errorf("failed to ensure service exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonServiceError, serviceErr.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, serviceErr
	}

	// Check if resources are fully ready (asynchronous readiness check)
	// For deployments, we check the Available condition and/or replica counts
	// For services, we just check if the Service object exists
	deploymentReady := sm.resourceManager.IsDeploymentAvailable(deployment)
	serviceReady := sm.resourceManager.IsServiceAvailable(service)

	// Apply access strategy when compute and service resources are ready
	if deploymentReady && serviceReady {
		// ReconcileAccess returns nil (no error) only when it successfully initiated
		// the creation of all AccessRessources.
		// TODO: add probe and requeue https://github.com/jupyter-infra/jupyter-k8s/issues/36
		if err := sm.ReconcileAccessForDesiredRunningStatus(ctx, workspace, service); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		// Then only update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")

		// Record workspace running event
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceRunning", "Workspace is now running")

		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace, snapshotStatus); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		// Handle idle shutdown for running workspaces
		return sm.handleIdleShutdownForRunningWorkspace(ctx, workspace)
	}

	// Resources are being created/started but not fully ready yet
	// Update status to Starting and requeue to check again later
	logger.Info("Resources not fully ready", "deploymentReady", deploymentReady, "serviceReady", serviceReady)
	workspace.Status.DeploymentName = deployment.GetName()
	workspace.Status.ServiceName = service.GetName()
	readiness := WorkspaceRunningReadiness{
		computeReady:         deploymentReady,
		serviceReady:         serviceReady,
		accessResourcesReady: false,
	}
	if err := sm.statusManager.UpdateStartingStatus(
		ctx, workspace, readiness, snapshotStatus); err != nil {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
}

// handleTemplateValidation validates the workspace's template reference and handles all validation outcomes.
// If validation fails or encounters a system error, it updates the workspace status and returns shouldContinue=false.
// On success, it returns the resolved template with shouldContinue=true.
func (sm *StateMachine) handleTemplateValidation(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	snapshotStatus *workspacesv1alpha1.WorkspaceStatus) (template *ResolvedTemplate, shouldContinue bool, err error) {
	logger := logf.FromContext(ctx)

	// No template reference - continue with default configuration
	if workspace.Spec.TemplateRef == nil {
		return nil, true, nil
	}

	validation, err := sm.templateResolver.ValidateAndResolveTemplate(ctx, workspace)
	if err != nil {
		// System error (couldn't fetch template, etc.)
		logger.Error(err, "Failed to validate template")
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return nil, false, err
	}

	if !validation.Valid {
		// Validation failed - policy enforced, stop reconciliation
		logger.Info("Validation failed, rejecting workspace", "violations", len(validation.Violations))

		// Record validation failure event
		templateName := *workspace.Spec.TemplateRef
		message := fmt.Sprintf("Validation failed for %s with %d violations", templateName, len(validation.Violations))
		sm.recorder.Event(workspace, corev1.EventTypeWarning, "ValidationFailed", message)

		if statusErr := sm.statusManager.SetInvalid(ctx, workspace, validation, snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update validation status")
		}
		// No error - successful policy enforcement
		return nil, false, nil
	}

	// Validation passed
	logger.Info("Validation passed")

	// Record successful validation event
	templateName := *workspace.Spec.TemplateRef
	message := "Validation passed for " + templateName
	sm.recorder.Event(workspace, corev1.EventTypeNormal, "Validated", message)

	return validation.Template, true, nil
}

// handleIdleShutdownForRunningWorkspace handles idle shutdown logic for running workspaces
func (sm *StateMachine) handleIdleShutdownForRunningWorkspace(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {

	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// If idle shutdown is not enabled, no requeue needed
	if workspace.Spec.IdleShutdown == nil || !workspace.Spec.IdleShutdown.Enabled {
		logger.V(1).Info("Idle shutdown not enabled")
		return ctrl.Result{}, nil
	}

	logger.Info("Processing idle shutdown", "timeout", workspace.Spec.IdleShutdown.TimeoutMinutes)

	// Check if it's time to poll (every 10 seconds)
	if sm.shouldCheckIdleNow(workspace) {
		logger.Info("Checking workspace idle status")

		if err := sm.checkAndUpdateIdleStatus(ctx, workspace); err != nil {
			logger.Error(err, "Failed to check idle status, will retry")
			// Continue polling on error
		}

		// Check if workspace should be stopped due to idle timeout
		if sm.shouldStopDueToIdle(ctx, workspace) {
			logger.Info("Workspace idle timeout reached, stopping workspace",
				"timeout", workspace.Spec.IdleShutdown.TimeoutMinutes)

			return sm.stopWorkspaceDueToIdle(ctx, workspace)
		}
	}

	// Always requeue after 10 seconds for next idle check
	return ctrl.Result{RequeueAfter: IdleCheckInterval}, nil
}

// checkAndUpdateIdleStatus checks idle endpoint and updates status
func (sm *StateMachine) checkAndUpdateIdleStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace) error {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// Check idle status
	idleResp, err := sm.idleChecker.CheckWorkspaceIdle(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to check workspace idle endpoint")
		return err
	}

	// Update status for debugging
	return sm.updateIdleStatusIfChanged(ctx, workspace, idleResp)
}

// shouldCheckIdleNow determines if we should check idle status now
func (sm *StateMachine) shouldCheckIdleNow(workspace *workspacesv1alpha1.Workspace) bool {
	// Always check if no previous check recorded
	if workspace.Status.IdleShutdown == nil || workspace.Status.IdleShutdown.LastChecked == nil {
		return true
	}

	// Check every 10 seconds
	return time.Since(workspace.Status.IdleShutdown.LastChecked.Time) >= IdleCheckInterval
}

// shouldStopDueToIdle determines if workspace should be stopped due to idle timeout
func (sm *StateMachine) shouldStopDueToIdle(ctx context.Context, workspace *workspacesv1alpha1.Workspace) bool {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	if workspace.Status.IdleShutdown == nil || workspace.Status.IdleShutdown.LastActivity == nil {
		logger.V(1).Info("No idle status available, not stopping")
		return false
	}

	timeout := time.Duration(workspace.Spec.IdleShutdown.TimeoutMinutes) * time.Minute
	idleTime := time.Since(workspace.Status.IdleShutdown.LastActivity.Time)

	if idleTime > timeout {
		logger.Info("Idle timeout reached", "idleTime", idleTime, "timeout", timeout)
		return true
	}

	logger.V(1).Info("Workspace still active, timeout not reached",
		"idleTime", idleTime,
		"timeout", timeout,
		"remaining", timeout-idleTime)
	return false
}

// stopWorkspaceDueToIdle stops the workspace due to idle timeout
func (sm *StateMachine) stopWorkspaceDueToIdle(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// Record event
	sm.recorder.Event(workspace, corev1.EventTypeNormal, "IdleShutdown",
		fmt.Sprintf("Stopping workspace due to idle timeout of %d minutes", workspace.Spec.IdleShutdown.TimeoutMinutes))

	// Update desired status to trigger stop
	workspace.Spec.DesiredStatus = "Stopped"
	if err := sm.resourceManager.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to update workspace desired status")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	logger.Info("Updated workspace desired status to Stopped")

	// Immediate requeue to start stopping process
	return ctrl.Result{RequeueAfter: 0}, nil
}

// updateIdleStatusIfChanged updates idle status only if it has changed
func (sm *StateMachine) updateIdleStatusIfChanged(ctx context.Context, workspace *workspacesv1alpha1.Workspace, idleResp *IdleResponse) error {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// Parse last activity time
	lastActivity, err := time.Parse(time.RFC3339, idleResp.LastActivity)
	if err != nil {
		logger.Error(err, "Failed to parse last activity time", "lastActivity", idleResp.LastActivity)
		return err
	}

	// Check if status needs updating
	needsUpdate := false
	now := metav1.NewTime(time.Now())

	if workspace.Status.IdleShutdown == nil {
		workspace.Status.IdleShutdown = &workspacesv1alpha1.IdleShutdownStatus{}
		needsUpdate = true
	}

	// Only update if values changed
	if workspace.Status.IdleShutdown.LastActivity == nil ||
		!workspace.Status.IdleShutdown.LastActivity.Time.Equal(lastActivity) {
		workspace.Status.IdleShutdown.LastActivity = &metav1.Time{Time: lastActivity}
		needsUpdate = true
	}

	// Always update these fields for debugging
	workspace.Status.IdleShutdown.LastChecked = &now
	workspace.Status.IdleShutdown.Enabled = workspace.Spec.IdleShutdown.Enabled
	workspace.Status.IdleShutdown.TimeoutMinutes = workspace.Spec.IdleShutdown.TimeoutMinutes

	if needsUpdate {
		logger.V(1).Info("Updating idle status", "lastActivity", lastActivity)
		return sm.statusManager.client.Status().Update(ctx, workspace)
	}

	return nil
}
