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

package v1alpha1

import (
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// +kubebuilder:webhook:path=/validate-pods-exec-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=pods/exec,verbs=create,versions=v1,name=vpods-exec-workspace-v1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

var podexeclog = logf.Log.WithName("pod-exec-webhook")

// PodExecValidator validates pod exec requests to ensure they only target workspace pods
// and originate from the controller service account
type PodExecValidator struct {
	client.Client
	controllerNamespace string
	decoder             *admission.Decoder
}

// Handle validates pod exec requests
func (v *PodExecValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	podexeclog.Info("Validating pod exec request",
		"pod", req.Name,
		"namespace", req.Namespace,
		"user", req.UserInfo.Username)

	// Check if request is from controller service account
	expectedUser := fmt.Sprintf("system:serviceaccount:%s:jupyter-k8s-controller-manager",
		v.controllerNamespace)

	if req.UserInfo.Username != expectedUser {
		podexeclog.Info("Denying exec from non-controller user",
			"user", req.UserInfo.Username,
			"expected", expectedUser)
		return admission.Denied("pod exec only allowed from controller service account")
	}

	// Get the target pod
	pod := &corev1.Pod{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, pod); err != nil {
		podexeclog.Error(err, "Failed to get pod for exec validation")
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("failed to get pod: %w", err))
	}

	// Check if pod has workspace label
	_, hasWorkspace := pod.Labels[controller.LabelWorkspaceName]
	if !hasWorkspace {
		podexeclog.Info("Denying controller exec to non-workspace pod",
			"pod", req.Name,
			"namespace", req.Namespace)
		return admission.Denied("controller can only exec into workspace pods")
	}

	podexeclog.Info("Allowing controller exec to workspace pod",
		"pod", req.Name,
		"workspace", pod.Labels[controller.LabelWorkspaceName])
	return admission.Allowed("controller exec to workspace pod allowed")
}

// InjectDecoder injects the decoder
func (v *PodExecValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// SetupPodExecWebhookWithManager registers the pod exec webhook with the manager
func SetupPodExecWebhookWithManager(mgr ctrl.Manager) error {
	// Get controller namespace from environment or use default
	controllerNamespace := "jupyter-k8s-system"
	if ns := mgr.GetConfig().Host; ns != "" {
		// Try to extract namespace from kubeconfig context if available
		// For now, use the default namespace
		controllerNamespace = "jupyter-k8s-system"
	}

	podExecValidator := &PodExecValidator{
		Client:              mgr.GetClient(),
		controllerNamespace: controllerNamespace,
	}

	mgr.GetWebhookServer().Register("/validate-pods-exec-workspace",
		&admission.Webhook{Handler: podExecValidator})
	return nil
}
