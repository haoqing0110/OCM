# Leader Label Manager

## Overview

The Leader Label Manager automatically manages a Kubernetes label on the Placement controller pod to identify which pod is currently the leader. This enables creating Services that route traffic only to the leader pod.

## How It Works

1. **Leader Election**: Placement controller uses Kubernetes leader election (via Lease objects)
2. **Label Management**: When a pod becomes the leader, it adds the label `cluster.open-cluster-management.io/leader: "true"` to itself
3. **Service Routing**: The debug Service uses this label in its selector to route only to the leader pod
4. **Automatic Cleanup**: When the pod loses leadership or terminates, the label is removed

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Placement Deployment (3 replicas)          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌──────────────────┐  ┌──────────────────┐           │
│  │   Follower Pod   │  │   Follower Pod   │           │
│  │  (no label)      │  │  (no label)      │           │
│  └──────────────────┘  └──────────────────┘           │
│                                                         │
│  ┌──────────────────────────────────────────────────┐  │
│  │              Leader Pod                          │  │
│  │  cluster.open-cluster-management.io/leader=true │  │
│  │                                                  │  │
│  │  ┌──────────────────────────────────────────┐   │  │
│  │  │  LeaderLabelManager                      │   │  │
│  │  │  • Adds label on leadership acquired     │   │  │
│  │  │  • Removes label on leadership lost      │   │  │
│  │  └──────────────────────────────────────────┘   │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                            ▲
                            │ Service selector:
                            │ cluster.open-cluster-management.io/leader=true
                            │
                ┌───────────────────────┐
                │  placement-debug      │
                │  Service              │
                │  (ClusterIP)          │
                └───────────────────────┘
```

## Usage

### Accessing the Debug API

Once deployed, you can access the debug endpoint via the Service:

```bash
# Get the service
kubectl get svc -n open-cluster-management

# Access debug API for a specific placement
kubectl exec -n open-cluster-management deployment/some-other-pod -- \
  curl -k https://cluster-manager-placement-debug:8443/debug/placements/default/my-placement

# Or port-forward the service
kubectl port-forward -n open-cluster-management \
  svc/cluster-manager-placement-debug 8443:8443

# Then access from localhost
curl -k https://localhost:8443/debug/placements/default/my-placement
```

### Verifying Leader Label

Check which pod is currently the leader:

```bash
# List pods with leader label
kubectl get pods -n open-cluster-management \
  -l cluster.open-cluster-management.io/leader=true

# Expected output (1 pod):
# NAME                                          READY   STATUS    RESTARTS   AGE
# cluster-manager-placement-controller-abc123   1/1     Running   0          5m
```

### Verifying Service Endpoints

Check that the Service only points to the leader pod:

```bash
# Get service endpoints
kubectl get endpoints -n open-cluster-management cluster-manager-placement-debug -o yaml

# Should show only 1 address (the leader pod IP)
```

## Implementation Details

### Environment Variables Required

The Deployment must set these environment variables:

```yaml
env:
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
```

### RBAC Permissions Required

The ServiceAccount needs permission to update Pod labels:

```yaml
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "update"]
```

### Leader Label Specification

- **Label Key**: `cluster.open-cluster-management.io/leader`
- **Label Value**: `"true"`
- **Applied To**: Current pod when it becomes leader
- **Removed**: When pod loses leadership or terminates

## Troubleshooting

### Service has no endpoints

**Symptom**: `kubectl get endpoints` shows no addresses

**Possible causes**:
1. No pod is currently the leader (check Lease object)
2. Leader label not applied (check pod labels)
3. Service selector mismatch

**Debug**:
```bash
# Check lease to see current leader
kubectl get lease -n open-cluster-management placement -o yaml

# Check pod labels
kubectl get pods -n open-cluster-management \
  -l app=clustermanager-placement-controller \
  --show-labels
```

### Multiple pods have leader label

**Symptom**: Multiple pods show the leader label

**This should not happen**, but if it does:
1. Check for split-brain in leader election
2. Verify leader election is working: `kubectl get lease`
3. Manually remove incorrect labels:
   ```bash
   kubectl label pod <pod-name> -n open-cluster-management \
     cluster.open-cluster-management.io/leader-
   ```

### Leader label not removed on pod termination

**Symptom**: Old leader still has the label after new leader is elected

**This is expected** - the label will remain on the old pod until:
- The pod is deleted (Kubernetes garbage collection)
- The pod restarts and loses leadership

**Mitigation**: The Service will automatically switch to the new leader as soon as it adds the label, so this doesn't affect functionality.

## Testing Leader Failover

Simulate leader failover to verify the Service switches correctly:

```bash
# 1. Note current leader
LEADER=$(kubectl get pods -n open-cluster-management \
  -l cluster.open-cluster-management.io/leader=true \
  -o jsonpath='{.items[0].metadata.name}')
echo "Current leader: $LEADER"

# 2. Delete the leader pod
kubectl delete pod -n open-cluster-management $LEADER

# 3. Wait for new leader election (usually < 137 seconds)
sleep 30

# 4. Check new leader
kubectl get pods -n open-cluster-management \
  -l cluster.open-cluster-management.io/leader=true

# 5. Verify Service endpoint switched to new leader
kubectl get endpoints -n open-cluster-management \
  cluster-manager-placement-debug -o yaml
```

## Code References

- **Label Manager**: `pkg/placement/leaderutil/label_manager.go`
- **Integration**: `pkg/placement/controllers/manager.go`
- **Service Manifest**: `manifests/cluster-manager/management/placement/service.yaml`
- **Deployment**: `manifests/cluster-manager/management/placement/deployment.yaml`
- **RBAC**: `manifests/cluster-manager/hub/placement/clusterrole.yaml`
