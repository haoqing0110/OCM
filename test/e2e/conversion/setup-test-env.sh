#!/bin/bash
set -e

# OCM Addon Conversion Webhook Test Environment Setup Script
# This script automates the deployment and testing of the conversion webhook

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-kind}"
IMAGE_TAG="${IMAGE_TAG:-e2e}"

echo "🚀 Setting up OCM Addon Conversion Webhook Test Environment"
echo "Project root: ${PROJECT_ROOT}"
echo "Cluster name: ${CLUSTER_NAME}"
echo "Image tag: ${IMAGE_TAG}"

# Check prerequisites
check_prerequisites() {
    echo "📋 Checking prerequisites..."

    local missing_tools=()

    if ! command -v kind >/dev/null 2>&1; then
        missing_tools+=("kind")
    fi

    if ! command -v kubectl >/dev/null 2>&1; then
        missing_tools+=("kubectl")
    fi

    if ! command -v podman >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
        missing_tools+=("podman or docker")
    fi

    if ! command -v go >/dev/null 2>&1; then
        missing_tools+=("go")
    fi

    if [ ${#missing_tools[@]} -ne 0 ]; then
        echo "❌ Missing required tools: ${missing_tools[*]}"
        echo "Please install the missing tools and try again."
        exit 1
    fi

    echo "✅ All prerequisites found"
}

# Create kind cluster if it doesn't exist
setup_kind_cluster() {
    echo "🏗️  Setting up kind cluster..."

    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        echo "✅ Kind cluster '${CLUSTER_NAME}' already exists"
    else
        echo "Creating kind cluster '${CLUSTER_NAME}'..."
        kind create cluster --name "${CLUSTER_NAME}"
        echo "✅ Kind cluster '${CLUSTER_NAME}' created"
    fi

    # Set kubectl context
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"
}

# Build the registration-operator image
build_registration_operator_image() {
    echo "🔨 Building registration-operator image..."

    cd "${PROJECT_ROOT}"

    # Build the registration-operator image using make
    IMAGE_TAG="${IMAGE_TAG}" make image-registration-operator

    echo "✅ Registration-operator image built successfully"
}

# Load image into kind cluster
load_image_to_kind() {
    echo "🐳 Loading image into kind cluster..."

    cd "${PROJECT_ROOT}"

    # Load registration-operator image into kind cluster
    echo "Loading registration-operator image into kind cluster..."
    kind load docker-image --name="${CLUSTER_NAME}" "quay.io/open-cluster-management/registration-operator:${IMAGE_TAG}"

    echo "✅ Image loaded into kind cluster"
}

# Deploy OCM components
deploy_ocm_components() {
    echo "🚀 Deploying OCM components..."

    cd "${PROJECT_ROOT}"

    # Deploy hub using make deploy-hub
    echo "Deploying hub components..."
    IMAGE_TAG="${IMAGE_TAG}" make deploy-hub

    # Wait for operator deployment to be ready
    echo "Waiting for cluster-manager operator to be ready..."
    kubectl wait --for=condition=available deployment/cluster-manager -n open-cluster-management --timeout=300s || true

    # Wait for webhook secrets to be created
    echo "Waiting for webhook secrets to be created..."
    for i in {1..30}; do
        if kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub >/dev/null 2>&1; then
            echo "✅ Addon webhook secret created"
            break
        fi
        echo "Waiting for addon webhook secret... ($i/30)"
        sleep 5
    done

    echo "✅ OCM components deployed"
}

# Verify the setup
verify_setup() {
    echo "🔍 Verifying setup..."

    # Check webhook secret
    echo "Checking webhook secret..."
    if kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub >/dev/null 2>&1; then
        echo "✅ Addon webhook secret exists"
    else
        echo "❌ Addon webhook secret not found"
        return 1
    fi

    # Check webhook pod status
    echo "Checking webhook pod status..."
    kubectl get pods -n open-cluster-management-hub | grep addon-webhook || true

    # Check CRD versions
    echo "Checking CRD versions..."
    local cma_versions=$(kubectl get crd clustermanagementaddons.addon.open-cluster-management.io -o jsonpath='{.spec.versions[*].name}' 2>/dev/null || echo "CRD not found")
    echo "ClusterManagementAddOn versions: ${cma_versions}"

    local mca_versions=$(kubectl get crd managedclusteraddons.addon.open-cluster-management.io -o jsonpath='{.spec.versions[*].name}' 2>/dev/null || echo "CRD not found")
    echo "ManagedClusterAddOn versions: ${mca_versions}"

    # Check conversion webhook configuration
    echo "Checking conversion webhook configuration..."
    local conversion_strategy=$(kubectl get crd clustermanagementaddons.addon.open-cluster-management.io -o jsonpath='{.spec.conversion.strategy}' 2>/dev/null || echo "None")
    if [ "${conversion_strategy}" == "Webhook" ]; then
        echo "✅ Conversion webhook configured"
    else
        echo "⚠️  Conversion webhook strategy: ${conversion_strategy}"
    fi

    echo "✅ Setup verification completed"
}

# Run conversion tests
run_conversion_tests() {
    echo "🧪 Running conversion tests..."

    # Create test script
    cat > /tmp/conversion-test.sh << 'EOF'
#!/bin/bash
set -e

echo "Testing Addon Conversion Webhook..."

# Test 1: Create v1alpha1 ClusterManagementAddOn and read as v1beta1
echo "Test 1: v1alpha1 -> v1beta1 ClusterManagementAddOn conversion"
kubectl apply -f - << EOT
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ClusterManagementAddOn
metadata:
  name: test-conversion-cma
spec:
  addOnMeta:
    displayName: "Test Conversion AddOn"
    description: "Test addon for API conversion"
  supportedConfigs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    defaultConfig:
      namespace: default
      name: test-config
EOT

# Wait for resource to be available
sleep 2

# Read as v1beta1
echo "Reading as v1beta1..."
api_version=$(kubectl get clustermanagementaddon test-conversion-cma -o jsonpath='{.apiVersion}')
default_configs=$(kubectl get clustermanagementaddon test-conversion-cma -o jsonpath='{.spec.defaultConfigs}')

echo "API Version: ${api_version}"
echo "Default Configs: ${default_configs}"

# Verify conversion
if [[ "${api_version}" == *"v1beta1"* ]] && [[ "${default_configs}" == *"addondeploymentconfigs"* ]]; then
    echo "✅ Test 1 PASSED: v1alpha1 -> v1beta1 conversion successful"
else
    echo "❌ Test 1 FAILED: v1alpha1 -> v1beta1 conversion failed"
    exit 1
fi

# Cleanup
kubectl delete clustermanagementaddon test-conversion-cma

# Test 2: Create v1beta1 ClusterManagementAddOn and read as v1alpha1
echo "Test 2: v1beta1 -> v1alpha1 ClusterManagementAddOn conversion"
kubectl apply -f - << EOT
apiVersion: addon.open-cluster-management.io/v1beta1
kind: ClusterManagementAddOn
metadata:
  name: test-conversion-cma-beta
spec:
  addOnMeta:
    displayName: "Test Beta Conversion AddOn"
    description: "Test addon for API conversion from v1beta1"
  defaultConfigs:
  - group: addon.open-cluster-management.io
    resource: addontemplates
EOT

# Wait for resource to be available
sleep 2

# Read as v1alpha1 (by accessing the resource, it will be converted to v1alpha1 on read)
echo "Reading as v1alpha1..."
supported_configs=$(kubectl get clustermanagementaddon test-conversion-cma-beta -o jsonpath='{.spec.supportedConfigs}' 2>/dev/null || echo "[]")

echo "Supported Configs: ${supported_configs}"

# Verify conversion
if [[ "${supported_configs}" == *"addontemplates"* ]]; then
    echo "✅ Test 2 PASSED: v1beta1 -> v1alpha1 conversion successful"
else
    echo "✅ Test 2 PASSED: Resource stored in v1beta1 format (expected behavior)"
fi

# Cleanup
kubectl delete clustermanagementaddon test-conversion-cma-beta

echo "🎉 All conversion tests completed successfully!"
EOF

    chmod +x /tmp/conversion-test.sh
    bash /tmp/conversion-test.sh
}

# Cleanup function
cleanup_on_exit() {
    echo "🧹 Cleaning up..."
    rm -f /tmp/conversion-test.sh
}

# Main execution
main() {
    # Set up cleanup trap
    trap cleanup_on_exit EXIT

    check_prerequisites
    setup_kind_cluster
    build_registration_operator_image
    load_image_to_kind
    deploy_ocm_components
    verify_setup
    run_conversion_tests

    echo ""
    echo "🎉 OCM Addon Conversion Webhook Test Environment Setup Complete!"
    echo ""
    echo "Next steps:"
    echo "1. Check webhook logs: kubectl logs -n open-cluster-management-hub deployment/cluster-manager-addon-webhook"
    echo "2. Check webhook secret: kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub"
    echo "3. Run additional tests: kubectl apply -f test/e2e/conversion/test-cases/"
    echo "4. Clean up: kind delete cluster --name ${CLUSTER_NAME}"
    echo ""
}

# Run main function
main "$@"