# PlacementDecision RollingUpdate Implementation Plan

## Overview

This document outlines the implementation plan for the PlacementDecision RollingUpdate strategy based on [enhancement 171](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/171-placementdecision-rollingupdate).

## Problem Statement

When clusters are redistributed across multiple PlacementDecisions (due to cluster additions/deletions or configuration changes), the current sequential update mechanism creates a time window where clusters temporarily disappear from all PlacementDecisions. This causes consumers (e.g., addon-management-controller) to unnecessarily delete and recreate resources, leading to workload disruption.

## Solution

Implement a **RollingUpdate strategy** with two phases:
- **Phase 1 (Surge)**: Update each PlacementDecision with the union of old and new clusters (temporary over-limit allowed)
- **Phase 2 (Finalize)**: Update each PlacementDecision to contain only the new clusters and delete obsolete PlacementDecisions

This ensures clusters remain visible in at least one PlacementDecision throughout the update process.

---

## Implementation Changes

### 1. API Changes (Depends on open-cluster-management.io/api)

**Note**: These API changes must be implemented in the [open-cluster-management.io/api](https://github.com/open-cluster-management-io/api) repository first, then vendored into this repository.

#### 1.1 Add UpdateStrategy to DecisionStrategy

**File**: `cluster/v1beta1/types_placement.go`

**Current Structure** (lines 161-166):
```go
// DecisionStrategy divide the created placement decision to groups and define number of clusters per decision group.
type DecisionStrategy struct {
    // groupStrategy defines strategies to divide selected clusters into decision groups.
    // +optional
    GroupStrategy GroupStrategy `json:"groupStrategy,omitempty"`
}
```

**Proposed Addition**:
```go
// DecisionStrategy divide the created placement decision to groups and define number of clusters per decision group.
type DecisionStrategy struct {
    // groupStrategy defines strategies to divide selected clusters into decision groups.
    // +optional
    GroupStrategy GroupStrategy `json:"groupStrategy,omitempty"`

    // updateStrategy defines how PlacementDecision objects should be updated when
    // the set of selected clusters changes. This controls the update mechanism for the decision
    // objects themselves, not how workloads are rolled out to clusters.
    // +optional
    UpdateStrategy UpdateStrategy `json:"updateStrategy,omitempty"`
}
```

#### 1.2 Add UpdateStrategy Types

**File**: `cluster/v1beta1/types_placement.go`

**Add after DecisionStrategy definition**:
```go
// UpdateStrategy defines how PlacementDecision objects should be updated.
type UpdateStrategy struct {
    // type indicates the type of the update strategy. Default is "All".
    // See UpdateStrategyType for detailed behavior of each type.
    //
    // +kubebuilder:validation:Enum=All;RollingUpdate
    // +kubebuilder:default=All
    // +optional
    Type UpdateStrategyType `json:"type,omitempty"`
}

// UpdateStrategyType is a string representation of the update strategy type
type UpdateStrategyType string

const (
    // All means update all PlacementDecisions immediately (current behavior).
    // Clusters may temporarily disappear from all decisions when moving between decisions,
    // which can cause consumers to incorrectly delete and recreate resources.
    UpdateStrategyTypeAll UpdateStrategyType = "All"

    // RollingUpdate means use a rolling update strategy similar to Deployment's RollingUpdate.
    // Phase 1 (Surge): Merge old and new clusters (temporary over-limit allowed)
    // Phase 2 (Finalize): Update to final state and delete obsolete PlacementDecisions
    // This ensures clusters remain visible in at least one decision throughout the update.
    UpdateStrategyTypeRollingUpdate UpdateStrategyType = "RollingUpdate"
)
```

#### 1.3 Update CRD Generation

After making API changes, regenerate CRDs:
```bash
cd /path/to/open-cluster-management.io/api
make update
make verify
```

---

### 2. Vendor API Changes (This Repository)

After API changes are merged and tagged in the `api` repository:

**Action**: Update vendor dependencies
```bash
cd /root/go/src/open-cluster-management-io/OCM
go get open-cluster-management.io/api@<new-version>
go mod tidy
go mod vendor
```

**Files Updated**:
- `vendor/open-cluster-management.io/api/cluster/v1beta1/types_placement.go`
- `vendor/open-cluster-management.io/api/cluster/v1beta1/zz_generated.deepcopy.go`

---

### 3. Scheduling Controller Implementation

#### 3.1 Add Rolling Update Logic

**File**: `pkg/placement/controllers/scheduling/scheduling_controller.go`

**Current bind() method** (lines 618-668):
```go
func (c *schedulingController) bind(
    ctx context.Context,
    placement *clusterapiv1beta1.Placement,
    placementdecisions []*clusterapiv1beta1.PlacementDecision,
    clusterScores PrioritizerScore,
    status *framework.Status,
) error {
    var errs []error
    placementDecisionNames := sets.NewString()

    // create/update placement decisions
    for _, pd := range placementdecisions {
        placementDecisionNames.Insert(pd.Name)
        err := c.createOrUpdatePlacementDecision(ctx, placement, pd, clusterScores, status)
        if err != nil {
            errs = append(errs, err)
        }
    }

    // query all placementdecisions of the placement
    requirement, err := labels.NewRequirement(clusterapiv1beta1.PlacementLabel, selection.Equals, []string{placement.Name})
    if err != nil {
        return err
    }
    labelSelector := labels.NewSelector().Add(*requirement)
    pds, err := c.placementDecisionLister.PlacementDecisions(placement.Namespace).List(labelSelector)
    if err != nil {
        return err
    }

    // delete redundant placementdecisions
    errs = []error{}
    for _, pd := range pds {
        if placementDecisionNames.Has(pd.Name) {
            continue
        }
        err := c.clusterClient.ClusterV1beta1().PlacementDecisions(
            pd.Namespace).Delete(ctx, pd.Name, metav1.DeleteOptions{})
        if errors.IsNotFound(err) {
            continue
        }
        if err != nil {
            errs = append(errs, err)
        }
        c.eventsRecorder.Eventf(
            placement, pd, corev1.EventTypeNormal,
            "DecisionDelete", "DecisionDeleted",
            "Decision %s is deleted with placement %s in namespace %s", pd.Name, placement.Name, placement.Namespace)
    }
    return errorhelpers.NewMultiLineAggregate(errs)
}
```

**Proposed Modification**:

Replace the `bind()` method with version-aware logic:

```go
func (c *schedulingController) bind(
    ctx context.Context,
    placement *clusterapiv1beta1.Placement,
    placementdecisions []*clusterapiv1beta1.PlacementDecision,
    clusterScores PrioritizerScore,
    status *framework.Status,
) error {
    // Determine update strategy
    updateStrategyType := clusterapiv1beta1.UpdateStrategyTypeAll
    if placement.Spec.DecisionStrategy.UpdateStrategy.Type != "" {
        updateStrategyType = placement.Spec.DecisionStrategy.UpdateStrategy.Type
    }

    // Route to appropriate implementation
    switch updateStrategyType {
    case clusterapiv1beta1.UpdateStrategyTypeRollingUpdate:
        return c.bindWithRollingUpdate(ctx, placement, placementdecisions, clusterScores, status)
    default:
        return c.bindAll(ctx, placement, placementdecisions, clusterScores, status)
    }
}

// bindAll implements the current "All" update strategy (immediate update)
func (c *schedulingController) bindAll(
    ctx context.Context,
    placement *clusterapiv1beta1.Placement,
    placementdecisions []*clusterapiv1beta1.PlacementDecision,
    clusterScores PrioritizerScore,
    status *framework.Status,
) error {
    // This is the existing bind() implementation (no changes)
    var errs []error
    placementDecisionNames := sets.NewString()

    // create/update placement decisions
    for _, pd := range placementdecisions {
        placementDecisionNames.Insert(pd.Name)
        err := c.createOrUpdatePlacementDecision(ctx, placement, pd, clusterScores, status)
        if err != nil {
            errs = append(errs, err)
        }
    }

    // query all placementdecisions of the placement
    requirement, err := labels.NewRequirement(clusterapiv1beta1.PlacementLabel, selection.Equals, []string{placement.Name})
    if err != nil {
        return err
    }
    labelSelector := labels.NewSelector().Add(*requirement)
    pds, err := c.placementDecisionLister.PlacementDecisions(placement.Namespace).List(labelSelector)
    if err != nil {
        return err
    }

    // delete redundant placementdecisions
    errs = []error{}
    for _, pd := range pds {
        if placementDecisionNames.Has(pd.Name) {
            continue
        }
        err := c.clusterClient.ClusterV1beta1().PlacementDecisions(
            pd.Namespace).Delete(ctx, pd.Name, metav1.DeleteOptions{})
        if errors.IsNotFound(err) {
            continue
        }
        if err != nil {
            errs = append(errs, err)
        }
        c.eventsRecorder.Eventf(
            placement, pd, corev1.EventTypeNormal,
            "DecisionDelete", "DecisionDeleted",
            "Decision %s is deleted with placement %s in namespace %s", pd.Name, placement.Name, placement.Namespace)
    }
    return errorhelpers.NewMultiLineAggregate(errs)
}

// bindWithRollingUpdate implements the RollingUpdate strategy
func (c *schedulingController) bindWithRollingUpdate(
    ctx context.Context,
    placement *clusterapiv1beta1.Placement,
    newPlacementDecisions []*clusterapiv1beta1.PlacementDecision,
    clusterScores PrioritizerScore,
    status *framework.Status,
) error {
    // Get all existing PlacementDecisions for this Placement
    requirement, err := labels.NewRequirement(clusterapiv1beta1.PlacementLabel, selection.Equals, []string{placement.Name})
    if err != nil {
        return err
    }
    labelSelector := labels.NewSelector().Add(*requirement)
    existingPDs, err := c.placementDecisionLister.PlacementDecisions(placement.Namespace).List(labelSelector)
    if err != nil {
        return err
    }

    // Build map of existing decisions by name
    existingPDMap := make(map[string]*clusterapiv1beta1.PlacementDecision)
    for _, pd := range existingPDs {
        existingPDMap[pd.Name] = pd
    }

    // Build map of new decisions by name
    newPDMap := make(map[string]*clusterapiv1beta1.PlacementDecision)
    for _, pd := range newPlacementDecisions {
        newPDMap[pd.Name] = pd
    }

    // --- Phase 1: Surge (merge old and new clusters) ---
    klog.V(4).Infof("RollingUpdate Phase 1 (Surge): merging old and new clusters for placement %s/%s",
        placement.Namespace, placement.Name)

    var phase1Errs []error
    for _, newPD := range newPlacementDecisions {
        existingPD, exists := existingPDMap[newPD.Name]
        if !exists {
            // New PlacementDecision - create it with new clusters
            err := c.createOrUpdatePlacementDecision(ctx, placement, newPD, clusterScores, status)
            if err != nil {
                phase1Errs = append(phase1Errs, err)
            }
            continue
        }

        // Existing PlacementDecision - merge old and new clusters
        mergedPD := newPD.DeepCopy()
        mergedClusters := mergePlacementDecisions(existingPD.Status.Decisions, newPD.Status.Decisions)
        mergedPD.Status.Decisions = mergedClusters

        // Log if we exceed the normal limit during surge
        if len(mergedClusters) > maxNumOfClusterDecisions {
            klog.V(2).Infof("RollingUpdate Phase 1: PlacementDecision %s/%s temporarily has %d clusters (exceeds normal limit of %d)",
                mergedPD.Namespace, mergedPD.Name, len(mergedClusters), maxNumOfClusterDecisions)
        }

        err := c.createOrUpdatePlacementDecision(ctx, placement, mergedPD, clusterScores, status)
        if err != nil {
            phase1Errs = append(phase1Errs, err)
        }
    }

    if len(phase1Errs) > 0 {
        return errorhelpers.NewMultiLineAggregate(phase1Errs)
    }

    // --- Phase 2: Finalize (set final state and delete obsolete decisions) ---
    klog.V(4).Infof("RollingUpdate Phase 2 (Finalize): updating to final state for placement %s/%s",
        placement.Namespace, placement.Name)

    var phase2Errs []error
    newPDNames := sets.NewString()

    // Update to final state
    for _, newPD := range newPlacementDecisions {
        newPDNames.Insert(newPD.Name)
        err := c.createOrUpdatePlacementDecision(ctx, placement, newPD, clusterScores, status)
        if err != nil {
            phase2Errs = append(phase2Errs, err)
        }
    }

    // Delete obsolete PlacementDecisions
    for _, existingPD := range existingPDs {
        if newPDNames.Has(existingPD.Name) {
            continue
        }
        err := c.clusterClient.ClusterV1beta1().PlacementDecisions(
            existingPD.Namespace).Delete(ctx, existingPD.Name, metav1.DeleteOptions{})
        if errors.IsNotFound(err) {
            continue
        }
        if err != nil {
            phase2Errs = append(phase2Errs, err)
        }
        c.eventsRecorder.Eventf(
            placement, existingPD, corev1.EventTypeNormal,
            "DecisionDelete", "DecisionDeleted",
            "Decision %s is deleted with placement %s in namespace %s", existingPD.Name, placement.Name, placement.Namespace)
    }

    return errorhelpers.NewMultiLineAggregate(phase2Errs)
}

// mergePlacementDecisions merges two lists of ClusterDecisions, removing duplicates
func mergePlacementDecisions(old, new []clusterapiv1beta1.ClusterDecision) []clusterapiv1beta1.ClusterDecision {
    clusterMap := make(map[string]clusterapiv1beta1.ClusterDecision)

    // Add old clusters
    for _, cluster := range old {
        clusterMap[cluster.ClusterName] = cluster
    }

    // Add new clusters (overwrites duplicates with new data)
    for _, cluster := range new {
        clusterMap[cluster.ClusterName] = cluster
    }

    // Convert back to slice
    merged := make([]clusterapiv1beta1.ClusterDecision, 0, len(clusterMap))
    for _, cluster := range clusterMap {
        merged = append(merged, cluster)
    }

    // Sort by cluster name for determinism
    sort.SliceStable(merged, func(i, j int) bool {
        return merged[i].ClusterName < merged[j].ClusterName
    })

    return merged
}
```

#### 3.2 Update createOrUpdatePlacementDecision

**File**: `pkg/placement/controllers/scheduling/scheduling_controller.go` (lines 672-755)

**Current Logic** (line 682-684):
```go
if len(clusterDecisions) > maxNumOfClusterDecisions {
    return fmt.Errorf("the number of clusterdecisions %q exceeds the max limitation %q", len(clusterDecisions), maxNumOfClusterDecisions)
}
```

**Proposed Change** (relax validation during RollingUpdate):
```go
// During RollingUpdate surge phase, we allow temporary over-limit
// Log as info instead of error
if len(clusterDecisions) > maxNumOfClusterDecisions {
    klog.V(2).Infof("PlacementDecision %s/%s has %d clusters (exceeds normal limit of %d). This may be expected during RollingUpdate surge phase.",
        placementDecision.Namespace, placementDecisionName, len(clusterDecisions), maxNumOfClusterDecisions)
}
```

**Alternative** (safer, recommended): Add context parameter to detect surge phase:
```go
func (c *schedulingController) createOrUpdatePlacementDecision(
    ctx context.Context,
    placement *clusterapiv1beta1.Placement,
    placementDecision *clusterapiv1beta1.PlacementDecision,
    clusterScores PrioritizerScore,
    status *framework.Status,
    allowOverLimit bool, // New parameter
) error {
    placementDecisionName := placementDecision.Name
    clusterDecisions := placementDecision.Status.Decisions

    if len(clusterDecisions) > maxNumOfClusterDecisions {
        if !allowOverLimit {
            return fmt.Errorf("the number of clusterdecisions %q exceeds the max limitation %q", len(clusterDecisions), maxNumOfClusterDecisions)
        }
        // Log as info during surge phase
        klog.V(2).Infof("PlacementDecision %s/%s temporarily has %d clusters during RollingUpdate surge (normal limit: %d)",
            placementDecision.Namespace, placementDecisionName, len(clusterDecisions), maxNumOfClusterDecisions)
    }
    // ... rest of implementation
}
```

Then update all callers:
- `bindAll()`: pass `allowOverLimit: false`
- `bindWithRollingUpdate()` Phase 1: pass `allowOverLimit: true`
- `bindWithRollingUpdate()` Phase 2: pass `allowOverLimit: false`

---

### 4. Unit Tests

#### 4.1 Test File Structure

**File**: `pkg/placement/controllers/scheduling/scheduling_controller_test.go`

**Add Test Cases**:

```go
// TestBindWithRollingUpdate_ClusterMove tests that clusters don't disappear when moving between decisions
func TestBindWithRollingUpdate_ClusterMove(t *testing.T) {
    // Setup: decision-1: [cluster1...cluster100], decision-2: [cluster101...cluster150]
    // Action: Add cluster0 -> cluster100 moves from decision-1 to decision-2
    // Expected:
    //   Phase 1: decision-1: [cluster0...cluster100] (101 clusters)
    //            decision-2: [cluster100...cluster150] (cluster100 in both)
    //   Phase 2: decision-1: [cluster0...cluster99]
    //            decision-2: [cluster100...cluster150]
}

// TestBindWithRollingUpdate_ClusterDelete tests rolling update during cluster deletion
func TestBindWithRollingUpdate_ClusterDelete(t *testing.T) {
    // Setup: decision-1: [cluster0...cluster99], decision-2: [cluster100...cluster150]
    // Action: Delete cluster0 -> cluster100 moves from decision-2 to decision-1
    // Expected: Similar two-phase update
}

// TestBindWithRollingUpdate_DecisionGroupChange tests ClustersPerDecisionGroup change
func TestBindWithRollingUpdate_DecisionGroupChange(t *testing.T) {
    // Setup: 250 clusters in 3 decisions (100+100+50)
    // Action: Change ClustersPerDecisionGroup from 100% to 50
    // Expected: Redistribute to 5 decisions without cluster disappearance
}

// TestBindAll_BackwardCompatibility tests that All strategy works as before
func TestBindAll_BackwardCompatibility(t *testing.T) {
    // Ensure existing behavior is unchanged when updateStrategy is not set or set to "All"
}

// TestMergePlacementDecisions tests the merge logic
func TestMergePlacementDecisions(t *testing.T) {
    tests := []struct {
        name     string
        old      []clusterapiv1beta1.ClusterDecision
        new      []clusterapiv1beta1.ClusterDecision
        expected []clusterapiv1beta1.ClusterDecision
    }{
        {
            name: "no overlap",
            old:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster1"}, {ClusterName: "cluster2"}},
            new:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster3"}, {ClusterName: "cluster4"}},
            expected: []clusterapiv1beta1.ClusterDecision{
                {ClusterName: "cluster1"}, {ClusterName: "cluster2"},
                {ClusterName: "cluster3"}, {ClusterName: "cluster4"},
            },
        },
        {
            name: "full overlap",
            old:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster1"}},
            new:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster1"}},
            expected: []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster1"}},
        },
        {
            name: "partial overlap",
            old:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster1"}, {ClusterName: "cluster2"}},
            new:  []clusterapiv1beta1.ClusterDecision{{ClusterName: "cluster2"}, {ClusterName: "cluster3"}},
            expected: []clusterapiv1beta1.ClusterDecision{
                {ClusterName: "cluster1"}, {ClusterName: "cluster2"}, {ClusterName: "cluster3"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := mergePlacementDecisions(tt.old, tt.new)
            // Assert got equals expected (sort both first)
        })
    }
}
```

---

### 5. Integration Tests

#### 5.1 Integration Test File

**File**: `test/integration/placement/placement_test.go`

**Add Test Cases**:

```go
// TestPlacementRollingUpdate_ClusterMovement tests end-to-end rolling update
func TestPlacementRollingUpdate_ClusterMovement(t *testing.T) {
    // 1. Create placement with updateStrategy.type: RollingUpdate
    // 2. Create 150 clusters -> verify 2 decisions created
    // 3. Add 1 cluster -> verify cluster doesn't disappear during redistribution
    // 4. Delete 1 cluster -> verify cluster doesn't disappear during redistribution
    // 5. Change ClustersPerDecisionGroup -> verify no cluster disappearance
}

// TestPlacementRollingUpdate_Events tests that events are properly recorded
func TestPlacementRollingUpdate_Events(t *testing.T) {
    // Verify events for both phases are recorded
}
```

---

### 6. E2E Tests

#### 6.1 E2E Test File

**File**: `test/e2e/placement_test.go`

**Add Test Cases**:

```go
// TestE2E_RollingUpdate_AddonStability tests that addons are not recreated during rolling update
func TestE2E_RollingUpdate_AddonStability(t *testing.T) {
    // 1. Create placement with RollingUpdate
    // 2. Deploy addon to selected clusters
    // 3. Add/remove clusters triggering PlacementDecision redistribution
    // 4. Verify addon UIDs don't change (no deletion/recreation)
}
```

---

### 7. Documentation Updates

#### 7.1 Code Comments

**Files to Update**:
- `pkg/placement/controllers/scheduling/scheduling_controller.go`: Add detailed comments explaining the rolling update strategy

#### 7.2 User Documentation (separate PR to website repo)

After implementation, update:
- [OCM website](https://open-cluster-management.io/) placement documentation
- Add examples of using RollingUpdate strategy
- Document the behavior differences between "All" and "RollingUpdate"

---

## Implementation Phases

### Phase 1: API Changes (Prerequisite)
**Repository**: `open-cluster-management.io/api`
- [ ] Add `UpdateStrategy` to `DecisionStrategy`
- [ ] Add `UpdateStrategyType` enum and constants
- [ ] Generate and verify CRDs
- [ ] Create PR and merge
- [ ] Tag new release

**Estimated Effort**: 1-2 days

### Phase 2: Controller Implementation
**Repository**: `open-cluster-management.io/OCM` (current)
- [ ] Vendor new API version
- [ ] Implement `bindWithRollingUpdate()`
- [ ] Implement `mergePlacementDecisions()`
- [ ] Update `bind()` to route based on strategy type
- [ ] Keep `bindAll()` for backward compatibility
- [ ] Update `createOrUpdatePlacementDecision()` to allow over-limit during surge

**Estimated Effort**: 3-4 days

### Phase 3: Testing
**Repository**: `open-cluster-management.io/OCM`
- [ ] Unit tests for merge logic
- [ ] Unit tests for rolling update phases
- [ ] Integration tests for end-to-end flow
- [ ] E2E tests with addon consumers

**Estimated Effort**: 3-4 days

### Phase 4: Documentation & Release
- [ ] Update code comments
- [ ] Create PR with all changes
- [ ] Code review and iteration
- [ ] Merge and tag release
- [ ] Update user documentation (website)

**Estimated Effort**: 2-3 days

---

## Rollback Plan

If issues are discovered:

1. **Default stays "All"**: Existing placements continue working unchanged
2. **Users can switch back**: Change `updateStrategy.type` from `RollingUpdate` to `All`
3. **Emergency fix**: If critical bug, revert the scheduling controller changes while keeping API (API is additive, safe to keep)

---

## Success Criteria

- [ ] Clusters never temporarily disappear from all PlacementDecisions during updates
- [ ] Addon UIDs remain stable during cluster redistribution
- [ ] Backward compatibility maintained (default behavior unchanged)
- [ ] All unit, integration, and E2E tests pass
- [ ] Documentation updated

---

## Open Questions

1. **Should we add metrics?**
   - Track rolling update phases
   - Measure duration of surge phase
   - Count temporary over-limit events

2. **Should we add a timeout?**
   - If Phase 1 or Phase 2 takes too long, should we fail or continue?

3. **Event granularity?**
   - Should we emit events for Phase 1 start/end and Phase 2 start/end?
   - Or just emit normal DecisionCreate/DecisionUpdate/DecisionDelete events?

---

## References

- Enhancement Proposal: `/root/go/src/open-cluster-management-io/enhancements/enhancements/sig-architecture/171-placementdecision-rollingupdate/README.md`
- Current Scheduling Controller: `pkg/placement/controllers/scheduling/scheduling_controller.go`
- Placement API: `vendor/open-cluster-management.io/api/cluster/v1beta1/types_placement.go`
- Similar Kubernetes Pattern: [Deployment RollingUpdate Strategy](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-update-deployment)
