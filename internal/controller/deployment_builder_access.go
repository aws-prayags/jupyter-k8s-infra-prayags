package controller

import (
	"bytes"
	"fmt"
	"text/template"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// partialAccessResourceData provides values for template substitutions
type partialAccessResourceData struct {
	Workspace      *workspacev1alpha1.Workspace
	AccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
}

// resolveAccessStrategyEnv interpolates the env defined in the AccessStrategy
// for a particular Workspace.
func (b *DeploymentBuilder) resolveAccessStrategyEnv(
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	workspace *workspacev1alpha1.Workspace,
) ([]map[string]string, error) {
	data := &partialAccessResourceData{
		Workspace:      workspace,
		AccessStrategy: accessStrategy,
	}

	var envVars = []map[string]string{}

	for _, envTemplate := range accessStrategy.Spec.MergeEnv {
		tmpl, err := template.New("env").Parse(envTemplate.ValueTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse env template for %s: %w", envTemplate.Name, err)
		}

		var value bytes.Buffer
		if err := tmpl.Execute(&value, data); err != nil {
			return nil, fmt.Errorf("failed to execute env template for %s: %w", envTemplate.Name, err)
		}

		envVars = append(envVars, map[string]string{
			"name":  envTemplate.Name,
			"value": value.String(),
		})
	}

	return envVars, nil
}

// AddAccessStrategyEnvToContainer adds environment variables from an access strategy to a container.
// The env vars defined in the AccessStrategy take precedence over those requested
// by the Workspace create / update API.
func (db *DeploymentBuilder) addAccessStrategyEnvToContainer(
	container *corev1.Container,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	if container == nil {
		return fmt.Errorf("container is nil, cannot apply env vars of AccessStrategy: %s", accessStrategy.Name)
	}

	resolvedAccessStrategyEnv, err := db.resolveAccessStrategyEnv(accessStrategy, workspace)
	if err != nil {
		return err
	}

	// Add each environment variable from the access strategy
	for _, resolvedEnvVar := range resolvedAccessStrategyEnv {
		name, ok := resolvedEnvVar["name"]
		if !ok {
			return fmt.Errorf("environment variable missing name: %s", name)
		}

		value, ok := resolvedEnvVar["value"]
		if !ok {
			return fmt.Errorf("environment variable %s missing value", name)
		}

		// Check if the env var already exists
		found := false
		for i, existing := range container.Env {
			if existing.Name == name {
				// Update the existing env var
				container.Env[i].Value = value
				found = true
				break
			}
		}

		// Add the env var if it doesn't exist
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
	}

	return nil
}

// ApplyAccessStrategyToDeployment applies access strategy settings to a container
// Currently only adds environment variables, but could be extended in the future
func (db *DeploymentBuilder) ApplyAccessStrategyToDeployment(
	deployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	if deployment == nil {
		return fmt.Errorf("cannot apply AccessStrategy '%s' to nil deployment", accessStrategy.Name)
	}
	if accessStrategy == nil {
		return nil // Nothing to do
	}
	primaryContainer := &deployment.Spec.Template.Spec.Containers[0]

	// Apply environment variables to primary container
	if err := db.addAccessStrategyEnvToContainer(primaryContainer, workspace, accessStrategy); err != nil {
		return fmt.Errorf("failed to add environment variables to container: %w", err)
	}

	// Apply deployment spec modifications if defined
	if err := db.applyDeploymentSpecModifications(deployment, accessStrategy); err != nil {
		return fmt.Errorf("failed to apply deployment spec modifications: %w", err)
	}

	return nil
}

// addSSMSidecarContainer adds an SSM sidecar container to the deployment with shared volume
func (db *DeploymentBuilder) addSSMSidecarContainer(
	deployment *appsv1.Deployment,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	// Create shared volume for communication between main container and sidecar
	sharedVolume := corev1.Volume{
		Name: "ssm-remote-access",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: func() *resource.Quantity {
					limit := resource.MustParse("1Gi")
					return &limit
				}(),
			},
		},
	}

	// Create volume mount for shared directory
	sharedVolumeMount := corev1.VolumeMount{
		Name:      "ssm-remote-access",
		MountPath: "/ssm-remote-access",
	}

	// Get sidecar image from access strategy controller configuration
	var sidecarImage string
	if accessStrategy.Spec.ControllerConfig != nil {
		sidecarImage = accessStrategy.Spec.ControllerConfig["SSM_SIDECAR_IMAGE"]
	}

	if sidecarImage == "" {
		return fmt.Errorf("SSM_SIDECAR_IMAGE environment variable not found in access strategy %s", accessStrategy.Name)
	}

	ssmContainer := corev1.Container{
		Name:    "ssm-agent-sidecar",
		Image:   sidecarImage,
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			"cp /usr/local/bin/remote-access-server /ssm-remote-access/ || echo \"Failed to copy: $?\"; sleep infinity",
		},
		VolumeMounts: []corev1.VolumeMount{sharedVolumeMount},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"test", "-f", "/tmp/ssm-registered"},
				},
			},
			InitialDelaySeconds: 2,
			PeriodSeconds:       2,
		},
	}

	// Add the shared volume to the deployment
	deployment.Spec.Template.Spec.Volumes = append(
		deployment.Spec.Template.Spec.Volumes,
		sharedVolume,
	)

	// Add volume mount to the main container (first container)
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		mainContainer := &deployment.Spec.Template.Spec.Containers[0]
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, sharedVolumeMount)
	}

	// Add the sidecar container to the deployment
	deployment.Spec.Template.Spec.Containers = append(
		deployment.Spec.Template.Spec.Containers,
		ssmContainer,
	)

	return nil
}

// applyDeploymentSpecModifications applies deployment modifications from access strategy
func (db *DeploymentBuilder) applyDeploymentSpecModifications(
	deployment *appsv1.Deployment,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	if accessStrategy.Spec.DeploymentSpecModifications == nil {
		return nil // Nothing to do
	}

	mods := accessStrategy.Spec.DeploymentSpecModifications
	
	// Add volumes
	if mods.PodSpec != nil && len(mods.PodSpec.Volumes) > 0 {
		logf.Log.V(1).Info("Adding volumes from deployment spec modifications", 
			"accessStrategy", accessStrategy.Name, 
			"volumeCount", len(mods.PodSpec.Volumes))
		
		deployment.Spec.Template.Spec.Volumes = append(
			deployment.Spec.Template.Spec.Volumes, 
			mods.PodSpec.Volumes...,
		)
		
		for _, volume := range mods.PodSpec.Volumes {
			logf.Log.V(1).Info("Added volume", 
				"accessStrategy", accessStrategy.Name,
				"volumeName", volume.Name)
		}
	}
	
	// Add volume mounts to primary container
	if mods.PrimaryContainer != nil && len(mods.PrimaryContainer.VolumeMounts) > 0 {
		logf.Log.V(1).Info("Adding volume mounts to primary container", 
			"accessStrategy", accessStrategy.Name, 
			"mountCount", len(mods.PrimaryContainer.VolumeMounts))
		
		primaryContainer := &deployment.Spec.Template.Spec.Containers[0]
		primaryContainer.VolumeMounts = append(
			primaryContainer.VolumeMounts, 
			mods.PrimaryContainer.VolumeMounts...,
		)
		
		for _, mount := range mods.PrimaryContainer.VolumeMounts {
			logf.Log.V(1).Info("Added volume mount to primary container", 
				"accessStrategy", accessStrategy.Name,
				"volumeName", mount.Name,
				"mountPath", mount.MountPath)
		}
	}
	
	// Add additional containers
	if mods.PodSpec != nil && len(mods.PodSpec.AdditionalContainers) > 0 {
		logf.Log.V(1).Info("Adding additional containers", 
			"accessStrategy", accessStrategy.Name, 
			"containerCount", len(mods.PodSpec.AdditionalContainers))
		
		deployment.Spec.Template.Spec.Containers = append(
			deployment.Spec.Template.Spec.Containers,
			mods.PodSpec.AdditionalContainers...,
		)
		
		for _, container := range mods.PodSpec.AdditionalContainers {
			logf.Log.V(1).Info("Added additional container", 
				"accessStrategy", accessStrategy.Name,
				"containerName", container.Name,
				"image", container.Image)
		}
	}
	
	logf.Log.Info("Successfully applied deployment spec modifications", 
		"accessStrategy", accessStrategy.Name)
	
	return nil
}
