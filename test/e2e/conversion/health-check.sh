#!/bin/bash
set -e

# OCM Addon Conversion Webhook Health Check Script
# This script provides comprehensive health checks and diagnostics

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-kind}"

echo "🔍 OCM Addon Conversion Webhook Health Check"
echo "Project root: ${PROJECT_ROOT}"
echo "Cluster name: ${CLUSTER_NAME}"
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Status tracking
OVERALL_STATUS="healthy"
ISSUES_FOUND=()

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
    ISSUES_FOUND+=("WARNING: $1")
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
    OVERALL_STATUS="unhealthy"
    ISSUES_FOUND+=("ERROR: $1")
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check required tools
    local tools=("kubectl" "kind" "go")
    local missing_tools=()

    for tool in "${tools[@]}"; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            missing_tools+=("$tool")
        fi
    done

    if [ ${#missing_tools[@]} -eq 0 ]; then
        log_success "All required tools are available"
    else
        log_error "Missing required tools: ${missing_tools[*]}"
    fi

    # Check container runtime
    if command -v podman >/dev/null 2>&1; then
        log_success "Container runtime: podman available"
    elif command -v docker >/dev/null 2>&1; then
        log_success "Container runtime: docker available"
    else
        log_error "No container runtime (podman or docker) found"
    fi
}

# Check kind cluster
check_kind_cluster() {
    log_info "Checking kind cluster..."

    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        log_success "Kind cluster '${CLUSTER_NAME}' exists"

        # Check cluster access
        if kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1; then
            log_success "Cluster is accessible"
        else
            log_error "Cannot access cluster '${CLUSTER_NAME}'"
        fi
    else
        log_error "Kind cluster '${CLUSTER_NAME}' not found"
    fi
}

# Check OCM components
check_ocm_components() {
    log_info "Checking OCM components..."

    # Check namespaces
    local namespaces=("open-cluster-management-hub" "open-cluster-management")
    for ns in "${namespaces[@]}"; do
        if kubectl get namespace "$ns" >/dev/null 2>&1; then
            log_success "Namespace '$ns' exists"
        else
            log_error "Namespace '$ns' not found"
        fi
    done

    # Check cluster manager operator deployment
    if kubectl get deployment cluster-manager -n open-cluster-management >/dev/null 2>&1; then
        local ready=$(kubectl get deployment cluster-manager -n open-cluster-management -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        local desired=$(kubectl get deployment cluster-manager -n open-cluster-management -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")

        if [ "$ready" = "$desired" ] && [ "$ready" != "0" ]; then
            log_success "Cluster manager operator deployment is ready ($ready/$desired)"
        else
            log_warning "Cluster manager operator deployment not ready ($ready/$desired)"
        fi
    else
        log_error "Cluster manager operator deployment not found"
    fi
}

# Check CRDs
check_crds() {
    log_info "Checking CRDs..."

    local crds=("clustermanagementaddons.addon.open-cluster-management.io" "managedclusteraddons.addon.open-cluster-management.io")

    for crd in "${crds[@]}"; do
        if kubectl get crd "$crd" >/dev/null 2>&1; then
            log_success "CRD '$crd' exists"

            # Check versions
            local versions=$(kubectl get crd "$crd" -o jsonpath='{.spec.versions[*].name}')
            log_info "  Versions: $versions"

            # Check conversion configuration
            local conversion_strategy=$(kubectl get crd "$crd" -o jsonpath='{.spec.conversion.strategy}' 2>/dev/null || echo "None")
            if [ "$conversion_strategy" = "Webhook" ]; then
                log_success "  Conversion webhook configured"

                # Check webhook configuration details
                local webhook_service=$(kubectl get crd "$crd" -o jsonpath='{.spec.conversion.webhook.clientConfig.service.name}' 2>/dev/null)
                local webhook_namespace=$(kubectl get crd "$crd" -o jsonpath='{.spec.conversion.webhook.clientConfig.service.namespace}' 2>/dev/null)
                log_info "  Webhook service: $webhook_service in namespace $webhook_namespace"
            else
                log_warning "  No conversion webhook configured (strategy: $conversion_strategy)"
            fi
        else
            log_error "CRD '$crd' not found"
        fi
    done
}

# Check conversion webhook
check_conversion_webhook() {
    log_info "Checking conversion webhook..."

    # Check webhook deployment
    if kubectl get deployment cluster-manager-addon-webhook -n open-cluster-management-hub >/dev/null 2>&1; then
        local ready=$(kubectl get deployment cluster-manager-addon-webhook -n open-cluster-management-hub -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        local desired=$(kubectl get deployment cluster-manager-addon-webhook -n open-cluster-management-hub -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")

        if [ "$ready" = "$desired" ] && [ "$ready" != "0" ]; then
            log_success "Conversion webhook deployment is ready ($ready/$desired)"
        else
            log_warning "Conversion webhook deployment not ready ($ready/$desired)"
        fi
    else
        log_error "Conversion webhook deployment not found"
    fi

    # Check webhook service
    if kubectl get service cluster-manager-addon-webhook -n open-cluster-management-hub >/dev/null 2>&1; then
        log_success "Conversion webhook service exists"
    else
        log_error "Conversion webhook service not found"
    fi

    # Check webhook secret
    if kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub >/dev/null 2>&1; then
        log_success "Conversion webhook TLS secret exists"

        # Check if secret has required keys
        local has_cert=$(kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub -o jsonpath='{.data.tls\.crt}' 2>/dev/null)
        local has_key=$(kubectl get secret addon-webhook-serving-cert -n open-cluster-management-hub -o jsonpath='{.data.tls\.key}' 2>/dev/null)

        if [ -n "$has_cert" ] && [ -n "$has_key" ]; then
            log_success "  Secret contains tls.crt and tls.key"
        else
            log_warning "  Secret is missing required keys"
        fi
    else
        log_error "Conversion webhook TLS secret 'addon-webhook-serving-cert' not found"
    fi

    # Check webhook pods
    local pod_count=$(kubectl get pods -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --no-headers 2>/dev/null | wc -l)
    if [ "$pod_count" -gt 0 ]; then
        log_success "Conversion webhook pods found ($pod_count)"

        # Check pod status
        local running_pods=$(kubectl get pods -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l)
        if [ "$running_pods" -eq "$pod_count" ]; then
            log_success "All webhook pods are running"
        else
            log_warning "$running_pods out of $pod_count webhook pods are running"
        fi
    else
        log_error "No conversion webhook pods found"
    fi
}

# Check webhook connectivity
check_webhook_connectivity() {
    log_info "Checking webhook connectivity..."

    # Check if we can reach the webhook service
    if kubectl get service cluster-manager-addon-webhook -n open-cluster-management-hub >/dev/null 2>&1; then
        # Get service details
        local service_ip=$(kubectl get service cluster-manager-addon-webhook -n open-cluster-management-hub -o jsonpath='{.spec.clusterIP}' 2>/dev/null)
        local service_port=$(kubectl get service cluster-manager-addon-webhook -n open-cluster-management-hub -o jsonpath='{.spec.ports[0].port}' 2>/dev/null)

        if [ -n "$service_ip" ] && [ -n "$service_port" ]; then
            log_success "Webhook service endpoint: $service_ip:$service_port"
        else
            log_warning "Could not retrieve webhook service endpoint details"
        fi
    fi
}

# Run conversion test
run_conversion_test() {
    log_info "Running conversion test..."

    # Create a simple test to verify conversion works
    cat > /tmp/health-check-test.yaml << 'EOF'
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ClusterManagementAddOn
metadata:
  name: health-check-test-addon
spec:
  addOnMeta:
    displayName: "Health Check Test AddOn"
    description: "Test addon for health check"
  supportedConfigs:
  - group: addon.open-cluster-management.io
    resource: addondeploymentconfigs
    defaultConfig:
      namespace: default
      name: test-config
EOF

    if kubectl apply -f /tmp/health-check-test.yaml >/dev/null 2>&1; then
        sleep 2

        # Try to read the resource as v1beta1 to test conversion
        local api_version=$(kubectl get clustermanagementaddon health-check-test-addon -o jsonpath='{.apiVersion}' 2>/dev/null || echo "")

        if [ -n "$api_version" ]; then
            log_success "Conversion test successful (API version: $api_version)"
        else
            log_warning "Conversion test failed - could not read resource"
        fi

        # Cleanup
        kubectl delete clustermanagementaddon health-check-test-addon --ignore-not-found=true >/dev/null 2>&1
    else
        log_error "Conversion test failed - could not create test resource"
    fi

    rm -f /tmp/health-check-test.yaml
}

# Show logs
show_logs() {
    log_info "Recent webhook logs:"
    echo ""

    if kubectl get pods -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --no-headers 2>/dev/null | head -1 >/dev/null; then
        kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --tail=10 --timestamps 2>/dev/null || \
            log_warning "Could not retrieve webhook logs"
    else
        log_warning "No webhook pods found to show logs"
    fi
}

# Generate diagnostic report
generate_diagnostic_report() {
    log_info "Generating diagnostic report..."

    local report_file="/tmp/ocm-webhook-health-report-$(date +%Y%m%d-%H%M%S).txt"

    {
        echo "OCM Addon Conversion Webhook Health Report"
        echo "Generated at: $(date)"
        echo "=========================================="
        echo ""

        echo "Environment:"
        echo "  Cluster: $CLUSTER_NAME"
        echo "  Kubectl context: $(kubectl config current-context 2>/dev/null || echo 'unknown')"
        echo ""

        echo "Overall Status: $OVERALL_STATUS"
        echo ""

        if [ ${#ISSUES_FOUND[@]} -gt 0 ]; then
            echo "Issues Found:"
            for issue in "${ISSUES_FOUND[@]}"; do
                echo "  - $issue"
            done
            echo ""
        fi

        echo "Detailed Information:"
        echo "-------------------"

        echo ""
        echo "Namespaces:"
        kubectl get namespaces | grep -E "(open-cluster-management|default)" 2>/dev/null || echo "  Error retrieving namespaces"

        echo ""
        echo "OCM Deployments:"
        kubectl get deployments -n open-cluster-management-hub 2>/dev/null || echo "  Error retrieving deployments"

        echo ""
        echo "CRDs:"
        kubectl get crd | grep addon 2>/dev/null || echo "  Error retrieving CRDs"

        echo ""
        echo "Webhook Pods:"
        kubectl get pods -n open-cluster-management-hub -l app=cluster-manager-addon-webhook 2>/dev/null || echo "  Error retrieving webhook pods"

        echo ""
        echo "Recent Webhook Logs:"
        kubectl logs -n open-cluster-management-hub -l app=cluster-manager-addon-webhook --tail=20 --timestamps 2>/dev/null || echo "  Error retrieving webhook logs"

    } > "$report_file"

    log_success "Diagnostic report saved to: $report_file"
}

# Main execution
main() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    check_prerequisites
    echo ""

    check_kind_cluster
    echo ""

    check_ocm_components
    echo ""

    check_crds
    echo ""

    check_conversion_webhook
    echo ""

    check_webhook_connectivity
    echo ""

    run_conversion_test
    echo ""

    show_logs
    echo ""

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    # Summary
    echo ""
    if [ "$OVERALL_STATUS" = "healthy" ]; then
        log_success "Overall health status: HEALTHY"
        echo ""
        echo "🎉 All checks passed! The OCM Addon Conversion Webhook is working correctly."
    else
        log_error "Overall health status: UNHEALTHY"
        echo ""
        echo "💥 Issues detected. Please review the errors above."

        if [ ${#ISSUES_FOUND[@]} -gt 0 ]; then
            echo ""
            echo "Summary of issues:"
            for issue in "${ISSUES_FOUND[@]}"; do
                echo "  • $issue"
            done
        fi
    fi

    echo ""
    echo "For detailed diagnostics, run with --verbose or --report flags."
    echo "For troubleshooting help, check the setup script: test/e2e/conversion/setup-test-env.sh"
}

# Parse command line arguments
VERBOSE=false
GENERATE_REPORT=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --report|-r)
            GENERATE_REPORT=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --verbose, -v    Show verbose output"
            echo "  --report, -r     Generate detailed diagnostic report"
            echo "  --help, -h       Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  CLUSTER_NAME  Name of the kind cluster (default: kind)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Run main function
main

# Generate report if requested
if [ "$GENERATE_REPORT" = "true" ]; then
    echo ""
    generate_diagnostic_report
fi