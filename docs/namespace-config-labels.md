# Namespace Configuration Labels

## Overview

The Tekton Pruner uses Kubernetes labels to identify and validate ConfigMaps. This ensures that only properly labeled ConfigMaps are processed by the pruner controllers and validated by the webhook.

## Required Labels

All pruner ConfigMaps **MUST** have the following labels:

### Common Labels (Required for All Configs)

```yaml
labels:
  app.kubernetes.io/part-of: tekton-pruner
```

### Config Type Label (Required)

```yaml
labels:
  pruner.tekton.dev/config-type: <type>
```

Where `<type>` is either:
- `global` - For cluster-wide configuration
- `namespace` - For namespace-specific configuration

## Configuration Types

### Global Configuration

**ConfigMap Name:** `tekton-pruner-default-spec`  
**Namespace:** `tekton-pipelines` (or your Tekton installation namespace)  
**Purpose:** Defines cluster-wide default pruning policies

**Example:**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-default-spec
  namespace: tekton-pipelines
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: global
    pruner.tekton.dev/release: "devel"
data:
  global-config: |
    enforcedConfigLevel: global
    ttlSecondsAfterFinished: 3600
    successfulHistoryLimit: 10
    failedHistoryLimit: 5
```

### Namespace Configuration

**ConfigMap Name:** `tekton-pruner-namespace-spec`  
**Namespace:** Any user namespace (your application namespace)  
**Purpose:** Overrides global settings for a specific namespace

**Example:**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-application
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  namespace-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 1800
    successfulHistoryLimit: 5
    failedHistoryLimit: 3
```

## Why Labels?

Labels provide several benefits:

1. **Filtering**: Controllers can efficiently filter ConfigMaps using label selectors
2. **Validation**: Webhook uses objectSelector to process only labeled ConfigMaps
3. **Type Safety**: Config type is explicitly declared via label
4. **Platform Agnostic**: Works consistently across all Kubernetes distributions
5. **No Side Effects**: Unlabeled ConfigMaps are completely ignored

## Validation Rules

The webhook validates:

1. ✓ Required labels are present
2. ✓ `config-type` label has valid value (`global` or `namespace`)
3. ✓ ConfigMap name matches expected pattern for config type
4. ✓ Global configs are in system namespace
5. ✓ Namespace configs are NOT in system namespaces
6. ✓ Configuration content is valid YAML
7. ✓ Namespace configs don't exceed global limits

## Creating a Namespace Configuration

To create a namespace-specific pruning configuration:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: tekton-pruner-namespace-spec
  namespace: my-namespace
  labels:
    app.kubernetes.io/part-of: tekton-pruner
    pruner.tekton.dev/config-type: namespace
data:
  namespace-config: |
    enforcedConfigLevel: namespace
    ttlSecondsAfterFinished: 1800
    successfulHistoryLimit: 5
EOF
```

## Troubleshooting

### ConfigMap Not Being Processed

If your ConfigMap is not being processed by the pruner:

1. Check labels are present and correct:
   ```bash
   kubectl get cm tekton-pruner-namespace-spec -n <namespace> -o jsonpath='{.metadata.labels}'
   ```

2. Verify the config-type label value:
   ```bash
   kubectl get cm tekton-pruner-namespace-spec -n <namespace> -o jsonpath='{.metadata.labels.pruner\.tekton\.dev/config-type}'
   ```

3. Check for validation errors:
   ```bash
   kubectl describe cm tekton-pruner-namespace-spec -n <namespace>
   ```

### Webhook Rejection

If the webhook rejects your ConfigMap:

- **Missing labels**: Add both `app.kubernetes.io/part-of` and `pruner.tekton.dev/config-type` labels
- **Wrong config-type**: Use `global` for cluster config, `namespace` for namespace config
- **Wrong name**: Use `tekton-pruner-default-spec` for global, `tekton-pruner-namespace-spec` for namespace
- **Wrong namespace**: Global configs must be in `tekton-pipelines`, namespace configs must NOT be in system namespaces

## Additional Resources

- [Getting Started Guide](./tutorials/getting-started.md)
- [Namespace Configuration Tutorial](./tutorials/namespace-configuration.md)
- [Configuration Reference](../config/600-tekton-pruner-default-spec.yaml)
