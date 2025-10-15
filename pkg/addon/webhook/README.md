# OCM Addon Webhooks

This package contains webhook implementations for Open Cluster Management (OCM) addon APIs, following the same structure pattern as `pkg/registration/webhook` and `pkg/work/webhook`.

## Structure

```
pkg/addon/webhook/
├── doc.go                          # Package documentation
├── start.go                        # Main webhook server setup
├── README.md                       # This documentation
└── conversion/                     # Conversion webhook implementation
    ├── doc.go                      # Package documentation
    ├── webhook.go                  # Webhook initialization structs
    ├── hub.go                      # Hub interface implementations
    ├── clustermanagementaddon_conversion.go   # CMA conversion logic
    ├── managedclusteraddon_conversion.go      # MCA conversion logic
    ├── managedclusteraddon_helpers.go         # MCA helper functions
    ├── helpers.go                  # Common conversion helpers
    └── conversion_test.go          # Comprehensive test suite
```

## Conversion Webhook

The conversion webhook enables bidirectional conversion between v1alpha1 and v1beta1 API versions for:
- `ClusterManagementAddOn`
- `ManagedClusterAddOn`

### Key Conversions

#### ClusterManagementAddOn
- **v1alpha1 → v1beta1**: `SupportedConfigs` field becomes `DefaultConfigs`
- **v1beta1 → v1alpha1**: `DefaultConfigs` field becomes `SupportedConfigs`

#### ManagedClusterAddOn
- **v1alpha1 → v1beta1**: `InstallNamespace` is preserved as annotation (deprecated in v1beta1)
- **v1beta1 → v1alpha1**: `InstallNamespace` is restored from annotation

### Hub Interface Implementation

Following the controller-runtime conversion pattern:

- **Hub types**: v1beta1 versions (storage versions)
  - `ClusterManagementAddOnV1Beta1`
  - `ManagedClusterAddOnV1Beta1`

- **Convertible types**: v1alpha1 versions
  - `ClusterManagementAddOnV1Alpha1`
  - `ManagedClusterAddOnV1Alpha1`

### Usage

The webhook is integrated into the addon manager command:

```bash
addon conversion-webhook --port=9443 --certdir=/tmp/certs
```

### Integration

To use this webhook package in your addon manager:

```go
import addonwebhook "open-cluster-management.io/ocm/pkg/addon/webhook"

func main() {
    webhookOptions := commonoptions.NewWebhookOptions()
    if err := addonwebhook.SetupWebhookServer(webhookOptions); err != nil {
        // handle error
    }
    webhookOptions.RunWebhookServer(ctrl.SetupSignalHandler())
}
```

### Pattern Compliance

This implementation follows the same pattern as other OCM webhook packages:

1. **Package structure**: Similar to `pkg/registration/webhook` and `pkg/work/webhook`
2. **Start function**: `SetupWebhookServer()` configures schemes and installs webhooks
3. **Webhook types**: Individual structs implementing `Init()` and `SetupWebhookWithManager()`
4. **Conversion interfaces**: Wrapper types implementing Hub and Convertible interfaces
5. **Testing**: Comprehensive test suite with conversion and roundtrip tests

### Testing

Run the conversion webhook tests:

```bash
go test ./pkg/addon/webhook/conversion/
```

The test suite includes:
- Bidirectional conversion tests
- Roundtrip conversion tests
- Hub interface implementation tests
- Edge case handling tests

### Backwards Compatibility

The conversion webhook maintains backwards compatibility by:

1. **Preserving deprecated fields**: `InstallNamespace` is stored as annotation
2. **Graceful field handling**: Missing fields get default values
3. **Metadata preservation**: Labels, annotations, and other metadata are maintained
4. **Status field mapping**: All status fields are properly converted between versions