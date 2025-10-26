# Idle Shutdown Template Examples

This directory contains comprehensive examples for testing idle shutdown template support.

## Directory Structure

```
idle-shutdown/
├── templates/           # WorkspaceTemplate examples
│   ├── flexible.yaml    # Template with configurable bounds
│   ├── locked.yaml      # Template with no overrides allowed
│   └── custom-endpoint.yaml # Template with custom detection endpoint
├── workspaces/          # Workspace examples
│   ├── inherit-example.yaml        # Inherits template config
│   ├── override-example.yaml       # Valid override within bounds
│   ├── locked-example.yaml         # Uses locked template
│   ├── violation-override-denied.yaml    # Should fail - override denied
│   └── violation-timeout-bounds.yaml     # Should fail - timeout out of bounds
└── kustomization.yaml   # Apply all valid examples at once
```

## Quick Start

### Apply All Valid Examples
```bash
# Apply all templates and valid workspaces
kubectl apply -k config/samples/idle-shutdown/

# Check that all resources are created
kubectl get workspacetemplates,workspaces
```

### Test Individual Scenarios

#### 1. Template Inheritance
```bash
kubectl apply -f templates/flexible.yaml
kubectl apply -f workspaces/inherit-example.yaml

# Verify workspace inherits template's 120-minute timeout
kubectl describe workspace workspace-idle-inherit
```

#### 2. Valid Override
```bash
kubectl apply -f templates/flexible.yaml
kubectl apply -f workspaces/override-example.yaml

# Verify workspace uses overridden 240-minute timeout
kubectl describe workspace workspace-idle-override
```

#### 3. Locked Template
```bash
kubectl apply -f templates/locked.yaml
kubectl apply -f workspaces/locked-example.yaml

# Verify workspace uses template's enforced 30-minute timeout
kubectl describe workspace workspace-locked-idle
```

#### 4. Custom Detection Endpoint
```bash
kubectl apply -f templates/custom-endpoint.yaml

# Check template uses custom endpoint /api/sessions on port 8080
kubectl describe workspacetemplate custom-endpoint-template
```

### Test Validation Errors

#### Override Denied Violation
```bash
kubectl apply -f templates/locked.yaml
kubectl apply -f workspaces/violation-override-denied.yaml

# Should fail with ViolationTypeIdleShutdownOverrideNotAllowed
kubectl get workspace workspace-idle-violation-override -o jsonpath='{.status.conditions[?(@.type=="Valid")]}'
```

#### Timeout Bounds Violation
```bash
kubectl apply -f templates/flexible.yaml
kubectl apply -f workspaces/violation-timeout-bounds.yaml

# Should fail with ViolationTypeIdleShutdownTimeoutOutOfBounds
kubectl get workspace workspace-idle-violation-bounds -o jsonpath='{.status.conditions[?(@.type=="Valid")]}'
```

## Template Configurations

### Flexible Template
- **Default timeout**: 120 minutes
- **Override policy**: Allowed with bounds (60-1440 minutes)
- **Detection**: Standard `/api/idle` on port 8888

### Locked Template
- **Default timeout**: 30 minutes
- **Override policy**: Not allowed (`allow: false`)
- **Detection**: Standard `/api/idle` on port 8888

### Custom Endpoint Template
- **Default timeout**: 180 minutes
- **Override policy**: Allowed with bounds (120-360 minutes)
- **Detection**: Custom `/api/sessions` on port 8080

## Debugging Commands

```bash
# Check workspace validation status
kubectl get workspaces -o custom-columns="NAME:.metadata.name,VALID:.status.conditions[?(@.type=='Valid')].status,REASON:.status.conditions[?(@.type=='Valid')].reason"

# Get detailed validation errors
kubectl get workspace <workspace-name> -o jsonpath='{.status.conditions[?(@.type=="Valid")].message}'

# Check controller logs
kubectl logs -n jupyter-k8s-system deployment/jupyter-k8s-controller-manager

# Describe template details
kubectl describe workspacetemplate <template-name>
```

## Cleanup

```bash
# Delete all idle shutdown examples
kubectl delete -k config/samples/idle-shutdown/

# Or delete individual resources
kubectl delete workspaces workspace-idle-inherit workspace-idle-override workspace-locked-idle
kubectl delete workspacetemplates flexible-idle-template security-idle-template custom-endpoint-template
```

## Expected Behavior

### ✅ Should Work
- Workspaces inheriting template idle shutdown config
- Valid workspace overrides within template bounds
- Locked templates preventing any overrides
- Custom detection endpoints from templates
- Non-template workspaces (backward compatibility)

### ❌ Should Fail with Validation Errors
- Workspace trying to override locked template
- Workspace timeout outside template bounds
- Invalid template references

## Integration with Existing Samples

These examples complement the existing idle shutdown samples in the main samples directory:
- `workspace_jupyter_idle_shutdown.yaml` - Non-template workspace (still works)
- `workspace_code_editor_idle_shutdown.yaml` - Non-template workspace (still works)