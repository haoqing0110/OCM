# OCM Addon Conversion Webhook Testing

This directory contains comprehensive testing infrastructure for the OCM (Open Cluster Management) Addon Conversion Webhook, which enables bidirectional conversion between v1alpha1 and v1beta1 API versions for ClusterManagementAddOn and ManagedClusterAddOn resources.

## Overview

The conversion webhook allows seamless migration between addon API versions, converting between:
- **v1alpha1 → v1beta1**: `SupportedConfigs` field becomes `DefaultConfigs`
- **v1beta1 → v1alpha1**: `DefaultConfigs` field becomes `SupportedConfigs`

## Quick Start

### Prerequisites

Ensure you have the following tools installed:
- `kind` (Kubernetes in Docker)
- `kubectl`
- `go` (1.21+)
- `podman` or `docker`

### Verified Deployment Steps

These steps have been tested and verified to work correctly:

```bash
# 1. Build the registration-operator image with e2e tag
IMAGE_TAG=e2e make image-registration-operator

# 2. Create a kind cluster
kind create cluster

# 3. Load the image into the kind cluster
kind load docker-image --name=kind quay.io/open-cluster-management/registration-operator:e2e

# 4. Deploy OCM hub components
IMAGE_TAG=e2e make deploy-hub
```

### Verify Deployment

```bash
# Wait for addon webhook secret to be created (should take ~30 seconds)
kubectl wait --for=jsonpath='{.data.tls\.crt}' secret/addon-webhook-serving-cert -n open-cluster-management-hub --timeout=300s

# Verify the secret exists with required keys
kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub -o jsonpath='{.data}' | jq 'keys'

# Check webhook pod is running
kubectl get pods -n open-cluster-management-hub -l app=cluster-manager-addon-webhook
```

### Run Health Check

```bash
# Basic health check
./test/e2e/conversion/health-check.sh

# Detailed health check with diagnostic report
./test/e2e/conversion/health-check.sh --report

# Verbose output
./test/e2e/conversion/health-check.sh --verbose
```

### Automated Setup

For automated deployment, use the setup script:

```bash
# Run automated setup (uses CLUSTER_NAME=kind and IMAGE_TAG=e2e by default)
./test/e2e/conversion/setup-test-env.sh

# Or with custom settings
CLUSTER_NAME=my-cluster IMAGE_TAG=custom ./test/e2e/conversion/setup-test-env.sh
```

This script will:
1. Check prerequisites
2. Create a kind cluster (if not exists)
3. Build the registration-operator image
4. Load the image into kind
5. Deploy OCM hub components
6. Wait for addon webhook secret creation
7. Run conversion tests

## Scripts Overview

### 1. `setup-test-env.sh`
**Main deployment script** that automates the complete setup:

```bash
./test/e2e/conversion/setup-test-env.sh
```

**Features:**
- Prerequisites checking
- Kind cluster creation
- Registration-operator image building
- Image loading into kind cluster
- OCM hub component deployment using `make deploy-hub`
- Automatic cert rotation for addon webhook TLS certificates
- Automated conversion testing

**Environment Variables:**
- `CLUSTER_NAME`: Kind cluster name (default: `kind`)
- `IMAGE_TAG`: Container image tag (default: `e2e`)

### 2. `health-check.sh`
**Comprehensive health verification** for the conversion webhook environment:

```bash
# Basic health check
./test/e2e/conversion/health-check.sh

# Detailed health check with report
./test/e2e/conversion/health-check.sh --report

# Verbose output
./test/e2e/conversion/health-check.sh --verbose
```

**Checks:**
- Prerequisites availability
- Kind cluster status
- OCM components health
- CRD configuration
- Webhook deployment status
- Pod health and logs
- Conversion functionality

### 3. Cleanup

To clean up the test environment:

```bash
# Delete the kind cluster (removes everything)
kind delete cluster --name=kind

# Or if using a custom cluster name
kind delete cluster --name=<your-cluster-name>
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kind Cluster                             │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │            open-cluster-management-hub                   │   │
│  │                                                         │   │
│  │  ┌─────────────────┐    ┌─────────────────────────┐    │   │
│  │  │ cluster-manager │    │ conversion-webhook      │    │   │
│  │  │ deployment      │    │ deployment              │    │   │
│  │  └─────────────────┘    │ - webhook server        │    │   │
│  │                         │ - TLS certificates      │    │   │
│  │                         │ - conversion logic      │    │   │
│  │                         └─────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                     CRDs                                │   │
│  │  ┌─────────────────────────────────────────────────┐   │   │
│  │  │ clustermanagementaddons.addon.ocm.io            │   │   │
│  │  │ - v1alpha1 (served)                             │   │   │
│  │  │ - v1beta1 (served, storage)                     │   │   │
│  │  │ - conversion webhook config                     │   │   │
│  │  └─────────────────────────────────────────────────┘   │   │
│  │  ┌─────────────────────────────────────────────────┐   │   │
│  │  │ managedclusteraddons.addon.ocm.io               │   │   │
│  │  │ - v1alpha1 (served)                             │   │   │
│  │  │ - v1beta1 (served, storage)                     │   │   │
│  │  │ - conversion webhook config                     │   │   │
│  │  └─────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Conversion Logic

### ClusterManagementAddOn Conversion

**v1alpha1 → v1beta1:**
```go
// v1alpha1
spec:
  supportedConfigs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    defaultConfig:
      namespace: default
      name: test-config

// Converts to v1beta1
spec:
  defaultConfigs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    namespace: default
    name: test-config
```

**v1beta1 → v1alpha1:**
```go
// v1beta1
spec:
  defaultConfigs:
  - group: addon.open-cluster-management.io
    resource: addontemplates

// Converts to v1alpha1
spec:
  supportedConfigs:
  - group: addon.open-cluster-management.io
    resource: addontemplates
```

## Testing Examples

### Basic Conversion Test

```bash
# Create v1alpha1 resource
kubectl apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ClusterManagementAddOn
metadata:
  name: test-conversion
spec:
  addOnMeta:
    displayName: "Test Conversion AddOn"
  supportedConfigs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    defaultConfig:
      namespace: default
      name: test-config
EOF

# Read as v1beta1 to verify conversion
kubectl get clustermanagementaddon test-conversion -o yaml

# Cleanup
kubectl delete clustermanagementaddon test-conversion
```

### Manual Webhook Testing

```bash
# Port forward to webhook service
kubectl port-forward svc/cluster-manager-addon-webhook -n open-cluster-management-hub 9443:9443 &

# Test readiness endpoint
curl -k https://localhost:9443/readyz

# Check webhook logs
kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --follow
```

## Certificate Rotation

The addon webhook uses automatic TLS certificate rotation managed by the cluster-manager operator:

### How It Works

1. **Cert Rotation Controller**: The cluster-manager operator includes a cert rotation controller that automatically creates and rotates TLS certificates
2. **Secret Creation**: On deployment, the controller creates the `addon-webhook-serving-cert` secret in the `open-cluster-management-hub` namespace
3. **Secret Contents**: The secret contains:
   - `tls.crt`: TLS certificate for the webhook service
   - `tls.key`: Private key for the certificate
4. **Automatic Rotation**: Certificates are automatically rotated before expiration
5. **CRD Integration**: The CA bundle is automatically injected into the CRD conversion webhook configuration

### Verification

```bash
# Check if secret exists
kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub

# View certificate details
kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout

# Check CA bundle in CRD
kubectl get crd clustermanagementaddons.addon.open-cluster-management.io -o jsonpath='{.spec.conversion.webhook.clientConfig.caBundle}' | base64 -d | openssl x509 -text -noout
```

## Common Issues and Solutions

### 1. Pod Stuck in ContainerCreating
**Problem:** Addon webhook pod shows "MountVolume.SetUp failed for volume 'webhook-secret': secret 'addon-webhook-serving-cert' not found"

**Root Cause:** Certificate rotation controller hasn't created the TLS secret yet

**Solution:**
```bash
# Wait for secret creation (usually takes 10-30 seconds)
kubectl wait --for=jsonpath='{.data.tls\.crt}' secret/addon-webhook-serving-cert -n open-cluster-management-hub --timeout=300s

# If timeout, check cluster-manager operator logs
kubectl logs -n open-cluster-management deployment/cluster-manager -f

# Verify RBAC permissions are correct (secret should be in resourceNames)
kubectl get clusterrole cluster-manager -o yaml | grep -A 20 "addon-webhook-serving-cert"
```

### 2. Conversion Not Working
**Problem:** Resources created in v1alpha1 don't convert to v1beta1 or vice versa

**Solution:**
```bash
# Check CRD conversion configuration
kubectl get crd clustermanagementaddons.addon.open-cluster-management.io -o jsonpath='{.spec.conversion}'

# Should show:
# {"strategy":"Webhook","webhook":{"clientConfig":{"caBundle":"...","service":{"name":"cluster-manager-addon-webhook","namespace":"open-cluster-management-hub","port":9443}},"conversionReviewVersions":["v1","v1beta1"]}}

# Check webhook pod logs for errors
kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --tail=50

# Verify webhook service is accessible
kubectl get svc cluster-manager-addon-webhook -n open-cluster-management-hub
```

### 3. ImagePullBackOff with e2e Tag
**Problem:** Some hub pods show ImagePullBackOff when using `IMAGE_TAG=e2e`

**Expected Behavior:** This is normal when using the e2e tag for local testing. The addon webhook pod should be running, which is what matters for conversion testing.

**Solution:**
```bash
# Only the addon webhook deployment needs to be ready for conversion testing
kubectl get deployment cluster-manager-addon-webhook -n open-cluster-management-hub

# If you need all components running, use published image tags
IMAGE_TAG=latest make deploy-hub
```

### 4. RBAC Permission Errors
**Problem:** Operator logs show "secrets 'addon-webhook-serving-cert' is forbidden"

**Root Cause:** The secret name is not in the ClusterRole's resourceNames list

**Solution:**
```bash
# Verify the secret is in the resourceNames list
kubectl get clusterrole cluster-manager -o yaml | grep -B 5 -A 5 "addon-webhook-serving-cert"

# If missing, the RBAC files need to be updated in:
# - deploy/cluster-manager/chart/cluster-manager/templates/cluster_role.yaml
# - deploy/cluster-manager/olm-catalog/latest/manifests/cluster-manager.clusterserviceversion.yaml
```

## Development Workflow

### 1. Making Changes to Conversion Logic

```bash
# Edit conversion files
vim pkg/addon/webhook/conversion/clustermanagementaddon_conversion.go

# Build and test locally
make build

# Run unit tests
make test-unit

# Deploy and test end-to-end
./test/e2e/conversion/setup-test-env.sh
```

### 2. Adding New Test Cases

```bash
# Add test cases to E2E test file
vim test/e2e/addon_conversion_webhook_test.go

# Or add to setup script for quick validation
vim test/e2e/conversion/setup-test-env.sh

# Run the updated tests
./test/e2e/conversion/setup-test-env.sh
```

### 3. Debugging Issues

```bash
# Generate diagnostic report
./test/e2e/conversion/health-check.sh --report

# Check webhook logs in real-time
kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --follow

# Port forward for direct testing
kubectl port-forward svc/cluster-manager-addon-webhook -n open-cluster-management-hub 9443:9443
```

## Integration with CI/CD

The conversion webhook tests can be integrated into CI/CD pipelines:

```bash
# Build image
IMAGE_TAG=e2e make image-registration-operator

# Create cluster
kind create cluster

# Load image
kind load docker-image --name=kind quay.io/open-cluster-management/registration-operator:e2e

# Deploy and test
IMAGE_TAG=e2e make deploy-hub

# Wait for webhook to be ready
kubectl wait --for=condition=available deployment/cluster-manager-addon-webhook -n open-cluster-management-hub --timeout=300s

# Run E2E tests
go test -v ./test/e2e -ginkgo.focus="addon conversion webhook"

# Cleanup
kind delete cluster
```

## Files and Components

```
test/e2e/conversion/
├── README.md                    # This documentation
├── setup-test-env.sh           # Automated setup script
└── health-check.sh             # Health verification script

test/e2e/
└── addon_conversion_webhook_test.go  # E2E test cases

pkg/addon/webhook/conversion/
├── webhook.go                  # HTTP webhook handler
├── clustermanagementaddon_conversion.go  # CMA conversion logic
├── managedclusteraddon_conversion.go     # MCA conversion logic
└── conversion_test.go          # Unit tests

pkg/operator/operators/clustermanager/controllers/certrotationcontroller/
└── certrotation_controller.go  # Certificate rotation logic

pkg/operator/helpers/
└── queuekey.go                 # Webhook secret/service constants

deploy/cluster-manager/
├── chart/cluster-manager/templates/cluster_role.yaml  # RBAC for Helm
└── olm-catalog/latest/manifests/cluster-manager.clusterserviceversion.yaml  # RBAC for OLM

Makefile                        # Make targets (image-registration-operator, deploy-hub)
```

## Support and Troubleshooting

For issues or questions:

1. **Check Health:** Run `./test/e2e/conversion/health-check.sh --report`
2. **View Operator Logs:** `kubectl logs -n open-cluster-management deployment/cluster-manager -f`
3. **View Webhook Logs:** `kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook -f`
4. **Clean Start:** `kind delete cluster && ./test/e2e/conversion/setup-test-env.sh`
5. **Check CRD Status:** `kubectl get crd clustermanagementaddons.addon.open-cluster-management.io -o yaml`

## Contributing

When contributing to the conversion webhook:

1. **Add Tests:** Update test cases in `test/e2e/addon_conversion_webhook_test.go`
2. **Update Documentation:** Keep this README current with any changes
3. **Test Thoroughly:** Run `./test/e2e/conversion/setup-test-env.sh` before submitting PRs
4. **Verify RBAC:** Ensure both Helm and OLM RBAC files are updated if adding new secrets
5. **Check Cert Rotation:** Verify cert rotation controller changes in `pkg/operator/operators/clustermanager/controllers/certrotationcontroller/`

## Key Implementation Details

### Certificate Rotation Integration

The addon webhook certificate is managed by the cert rotation controller:

**File:** `pkg/operator/operators/clustermanager/controllers/certrotationcontroller/certrotation_controller.go`

- Informer watches `addon-webhook-serving-cert` secret
- Target rotation creates cert for hostname: `cluster-manager-addon-webhook.open-cluster-management-hub.svc`
- Automatic cleanup on ClusterManager deletion

**File:** `pkg/operator/operators/clustermanager/options.go`

- Creates one-term informer filtered by secret name using FieldSelector
- Starts informer in operator startup

### RBAC Configuration

Both Helm and OLM deployments require `addon-webhook-serving-cert` in resourceNames:

**Files:**
- `deploy/cluster-manager/chart/cluster-manager/templates/cluster_role.yaml` (line 25)
- `deploy/cluster-manager/olm-catalog/latest/manifests/cluster-manager.clusterserviceversion.yaml` (line 151)

### CRD Version Strategy

Both ClusterManagementAddOn and ManagedClusterAddOn CRDs use:
- **v1beta1**: Served=true, Storage=true (hub version)
- **v1alpha1**: Served=true, Storage=false (backward compatibility)
- **Conversion**: Webhook strategy pointing to `cluster-manager-addon-webhook` service

This testing infrastructure provides a comprehensive foundation for developing, testing, and maintaining the OCM Addon Conversion Webhook with confidence and reliability.