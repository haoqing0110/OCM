package debugger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterinformerv1beta1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1beta1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
	clusterlisterv1beta1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1beta1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"open-cluster-management.io/ocm/pkg/placement/controllers/scheduling"
)

const DebugPath = "/debug/placements/"

// Debugger provides a debug http endpoint for scheduler
type Debugger struct {
	scheduler       scheduling.Scheduler
	clusterLister   clusterlisterv1.ManagedClusterLister
	placementLister clusterlisterv1beta1.PlacementLister
}

// PlacementWithoutStatus contains the placement information without status
type PlacementWithoutStatus struct {
	APIVersion string                       `json:"apiVersion,omitempty"`
	Kind       string                       `json:"kind,omitempty"`
	Metadata   map[string]interface{}       `json:"metadata,omitempty"`
	Spec       clusterv1beta1.PlacementSpec `json:"spec,omitempty"`
}

// DebugResult is the result returned by debugger
type DebugResult struct {
	Placement         *PlacementWithoutStatus        `json:"placement,omitempty"`
	FilterResults     []scheduling.FilterResult      `json:"filteredPiplieResults,omitempty"`
	PrioritizeResults []scheduling.PrioritizerResult `json:"prioritizeResults,omitempty"`
	PrioritizerScores scheduling.PrioritizerScore    `json:"prioritizerScores,omitempty"`
	Error             string                         `json:"error,omitempty"`
}

func NewDebugger(
	scheduler scheduling.Scheduler,
	placementInformer clusterinformerv1beta1.PlacementInformer,
	clusterInformer clusterinformerv1.ManagedClusterInformer) *Debugger {
	return &Debugger{
		scheduler:       scheduler,
		clusterLister:   clusterInformer.Lister(),
		placementLister: placementInformer.Lister(),
	}
}

func (d *Debugger) Handler(w http.ResponseWriter, r *http.Request) {
	var placement *clusterv1beta1.Placement
	var err error

	// Support both GET (fetch from API) and POST (accept JSON body)
	if r.Method == http.MethodPost {
		// POST: Parse Placement from request body
		placement, err = d.parsePlacementFromBody(r)
		if err != nil {
			d.reportErr(w, err)
			return
		}
	} else {
		// GET: Fetch Placement from API (original behavior)
		namespace, name, err := d.parsePath(r.URL.Path)
		if err != nil {
			d.reportErr(w, err)
			return
		}

		placement, err = d.placementLister.Placements(namespace).Get(name)
		if err != nil {
			d.reportErr(w, err)
			return
		}
	}

	clusters, err := d.clusterLister.List(labels.Everything())
	if err != nil {
		d.reportErr(w, err)
		return
	}

	scheduleResults, _ := d.scheduler.Schedule(r.Context(), placement, clusters)

	// Create placement without status
	placementWithoutStatus := &PlacementWithoutStatus{
		APIVersion: placement.APIVersion,
		Kind:       placement.Kind,
		Metadata: map[string]interface{}{
			"name":      placement.Name,
			"namespace": placement.Namespace,
		},
		Spec: placement.Spec,
	}
	// Add labels if present
	if len(placement.Labels) > 0 {
		placementWithoutStatus.Metadata["labels"] = placement.Labels
	}
	// Add annotations if present
	if len(placement.Annotations) > 0 {
		placementWithoutStatus.Metadata["annotations"] = placement.Annotations
	}

	result := DebugResult{
		Placement:         placementWithoutStatus,
		FilterResults:     scheduleResults.FilterResults(),
		PrioritizeResults: scheduleResults.PrioritizerResults(),
		PrioritizerScores: scheduleResults.PrioritizerScores(),
	}

	resultByte, _ := json.Marshal(result)

	_, _ = w.Write(resultByte)
}

func (d *Debugger) parsePath(path string) (string, string, error) {
	metaNamespaceKey := strings.TrimPrefix(path, DebugPath)
	return cache.SplitMetaNamespaceKey(metaNamespaceKey)
}

func (d *Debugger) parsePlacementFromBody(r *http.Request) (*clusterv1beta1.Placement, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var placement clusterv1beta1.Placement
	if err := json.Unmarshal(body, &placement); err != nil {
		return nil, fmt.Errorf("failed to unmarshal placement JSON: %w", err)
	}

	return &placement, nil
}

func (d *Debugger) reportErr(w http.ResponseWriter, err error) {
	result := &DebugResult{Error: err.Error()}

	resultByte, _ := json.Marshal(result)

	_, _ = w.Write(resultByte)
}
