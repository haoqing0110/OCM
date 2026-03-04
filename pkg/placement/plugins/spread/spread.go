package spread

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"

	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	clusterapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"open-cluster-management.io/ocm/pkg/placement/controllers/framework"
	"open-cluster-management.io/ocm/pkg/placement/plugins"
)

const (
	placementLabel = "cluster.open-cluster-management.io/placement"
	description    = `
	Spread selector selects the clusters from different topologies to make the workload
	spreads evenly to the topologies.
	`
)

var _ plugins.Selector = &Spread{}

type Spread struct {
	weight float64 // the weight from prioritizerPolicy
}

func New(weight int32) *Spread {
	return &Spread{
		weight: float64(weight),
	}
}

func (s *Spread) Name() string {
	return reflect.TypeOf(*s).Name()
}

func (s *Spread) Description() string {
	return description
}

func (s *Spread) Select(ctx context.Context, placement *clusterapiv1beta1.Placement, scores map[string]int64, clusters []*clusterapiv1.ManagedCluster) (plugins.PluginSelectResult, *framework.Status) {
	spreadConstraints := placement.Spec.SpreadPolicy.SpreadConstraints

	//	// spreadConstraints is sorted to ensure the constraint with a higher weight are
	//	// earlier accessed when iterating.
	//	sort.Slice(spreadConstraints, func(i, j int) bool {
	//		if spreadConstraints[i].Weight == spreadConstraints[j].Weight {
	//			return spreadConstraints[i].TopologyKey < spreadConstraints[j].TopologyKey
	//		}
	//		return spreadConstraints[i].Weight > spreadConstraints[j].Weight
	//	})

	clusterSet, clusterTopologies := filterClustersBySpreadConstraints(&spreadConstraints, clusters)
	topologyRoot := buildTopologyHierarchy(&spreadConstraints, clusterSet, clusterTopologies)

	var expectedNumOfClusters int32
	if placement.Spec.NumberOfClusters == nil {
		expectedNumOfClusters = int32(len(clusters))
	} else {
		expectedNumOfClusters = *placement.Spec.NumberOfClusters
	}

	var results []*clusterapiv1.ManagedCluster
	var selectedNumOfClusters int32

	// In each iteration, select one cluster
	for selectedNumOfClusters < expectedNumOfClusters && len(clusterSet) != 0 {
		selected, err := s.selectOne(&spreadConstraints, clusterSet, clusterTopologies, topologyRoot, scores)
		if err != nil {
			return plugins.PluginSelectResult{}, framework.NewStatus("Spread", framework.Warning, err.Error())
		}
		results = append(results, selected)
		delete(clusterSet, selected.Name)
		updateTopologyHierarchy(topologyRoot, clusterTopologies[selected.Name], &spreadConstraints)
		selectedNumOfClusters++
	}
	return plugins.PluginSelectResult{Selected: results}, framework.NewStatus("", framework.Success, "")
}

func (s *Spread) RequeueAfter(ctx context.Context, placement *clusterapiv1beta1.Placement) (plugins.PluginRequeueResult, *framework.Status) {
	return plugins.PluginRequeueResult{}, framework.NewStatus(s.Name(), framework.Success, "")
}

// selectOne returns one selected cluster each time.
func (s *Spread) selectOne(spreadConstraints *[]clusterapiv1beta1.SpreadConstraintsTerm, clusterSet map[string]*clusterapiv1.ManagedCluster,
	clusterTopologies map[string]TopologyMapping, topologyRoot *topologyNode, prioritizerScores map[string]int64) (*clusterapiv1.ManagedCluster, error) {
	spreadScores := make(map[string]uint64)
	// the number of clusters that are excluded due to the violation to max skew
	deleteCount := 0

	for _, c := range clusterSet {
		node := topologyRoot
		mapping := clusterTopologies[c.Name]
		spreadScores[c.Name] = 0
		for _, constraint := range *spreadConstraints {
			topologyName := mapping[getTopologyFullKey(&constraint)]
			nextNode := node.children[topologyName]

			// check the maxSkew constraint for the cluster if present
			if constraint.MaxSkew >= 1 {
				skewIfSelected := nextNode.selectedCount + 1 - node.minChildrenSelectedCount
				if skewIfSelected > constraint.MaxSkew {
					delete(spreadScores, c.Name)
					deleteCount++

					if deleteCount == len(clusterSet) {
						return nil, errors.New(fmt.Sprintf("maxSkew on %s cannot be satisfied", constraint.TopologyKey))
					}
					break
				}
			}

			// The spread score of each cluster is a 64-bit unsigned int,
			// where the score calculated from each spread constraint occupies 6 bits (i.e., [0, 63]).
			// The score calculated from a spread constraint with higher weight appear in higher bits.
			spreadScores[c.Name] <<= 6
			deltaChildrenSelectedCount := node.maxChildrenSelectedCount - node.minChildrenSelectedCount
			if deltaChildrenSelectedCount != 0 {
				// Clusters in a topology which have less already selected clusters have
				// higher scores.
				s := float64(node.maxChildrenSelectedCount-nextNode.selectedCount) /
					float64(deltaChildrenSelectedCount)
				// Normalize the score to [0, 63] and add it to the 64-bit spread score.
				spreadScores[c.Name] += uint64(s * 63)
			}
			node = nextNode
		}
	}

	minSpreadScore := uint64(math.MaxUint64)
	maxSpreadScore := uint64(0)
	for _, spreadScore := range spreadScores {
		if spreadScore < minSpreadScore {
			minSpreadScore = spreadScore
		}
		if spreadScore > maxSpreadScore {
			maxSpreadScore = spreadScore
		}
	}
	deltaSpreadScore := float64(maxSpreadScore - minSpreadScore)
	maxFinalScore := -math.MaxFloat64
	var maxFinalScoreCluster *clusterapiv1.ManagedCluster
	for clusterName, score := range spreadScores {
		spreadScore := float64(0)
		if deltaSpreadScore != 0 {
			// normalize the spread score to [-100, 100]
			spreadScore = float64(score-minSpreadScore)/deltaSpreadScore*200 - 100
		}
		// Sum the spread score with the scores from prioritizers, and select the cluster
		// with minimum final score.
		finalScore := float64(prioritizerScores[clusterName]) + spreadScore*s.weight
		if finalScore > maxFinalScore ||
			finalScore == maxFinalScore && clusterName < maxFinalScoreCluster.Name {
			maxFinalScore = finalScore
			maxFinalScoreCluster = clusterSet[clusterName]
		}
	}
	return maxFinalScoreCluster, nil
}

// buildTopologyHierarchy returns a root topologyNode representing a tree hierarchy of the topologies.
// The layers in the hierarchy represent the topology keys in spreadConstraints, where a topology key associated with a
// spread constraint of a higher weight corresponds to a layer closer to the root topologyNode. In this hierarchy,
// the number of already selected clusters of the topologies are maintained to support selectOne.
//
// E.g., our clusters are organized by a two layer hierarchy (provider and region). We have two providers, aws and azure.
// In each provider, we have two regions (us-east and us-west). Then, the tree hierarchy returned by buildTopologyHierarchy
// will be like:
//
// root node
// ├── aws node
// │   ├── us-east node
// │   └── us-west node
// └── azure node
//
//	├── us-east node (different from the us-east node under the aws node)
//	└── us-west node (different from the us-west node under the aws node)
func buildTopologyHierarchy(spreadConstraints *[]clusterapiv1beta1.SpreadConstraintsTerm, clusterSet map[string]*clusterapiv1.ManagedCluster, clusterTopologies map[string]TopologyMapping) *topologyNode {
	topologyRoot := newTopologyNode()
	for _, cluster := range clusterSet {
		node := topologyRoot
		topologyMapping := clusterTopologies[cluster.Name]
		for _, constraint := range *spreadConstraints {
			topologyName := topologyMapping[getTopologyFullKey(&constraint)]
			if _, ok := node.children[topologyName]; !ok {
				node.children[topologyName] = newTopologyNode()
			}
			node = node.children[topologyName]
		}
	}
	return topologyRoot
}

// filterClustersBySpreadConstraints filters out the clusters that do not have all the topology keys specified by spreadConstraints,
// it returns the remaining clusters and their TopologyMappings.
func filterClustersBySpreadConstraints(spreadConstraints *[]clusterapiv1beta1.SpreadConstraintsTerm, clusters []*clusterapiv1.ManagedCluster) (map[string]*clusterapiv1.ManagedCluster, map[string]TopologyMapping) {
	clusterSet := make(map[string]*clusterapiv1.ManagedCluster)
	clusterTopologies := make(map[string]TopologyMapping)

out:
	for _, cluster := range clusters {
		mapping := make(TopologyMapping)
		for _, constraint := range *spreadConstraints {
			topologyName := getTopologyName(cluster, getTopologyFullKey(&constraint))
			if topologyName == "" {
				continue out
			}
			mapping[getTopologyFullKey(&constraint)] = topologyName
		}
		clusterSet[cluster.Name] = cluster
		clusterTopologies[cluster.Name] = mapping
	}

	return clusterSet, clusterTopologies
}

// getTopologyName return the topology name of the cluster corresponding to the topology full key.
func getTopologyName(cluster *clusterapiv1.ManagedCluster, fullKey topologyFullKey) string {
	var labelsOrClaims map[string]string
	if fullKey.topologyKeyType == clusterapiv1beta1.TopologyKeyTypeClaim {
		labelsOrClaims = make(map[string]string)
		for _, claim := range cluster.Status.ClusterClaims {
			labelsOrClaims[claim.Name] = claim.Value
		}
	} else { // topologyKeyType == clusterapiv1beta1.TopologyKeyTypeLabel
		labelsOrClaims = cluster.Labels
	}
	return labelsOrClaims[fullKey.topologyKey]
}

// updateSelectedCount updates the selected counts in the tree of topologyNode.
func updateTopologyHierarchy(topologyRoot *topologyNode, selectedTopologyMapping TopologyMapping, spreadConstraints *[]clusterapiv1beta1.SpreadConstraintsTerm) {
	node := topologyRoot
	for _, constraint := range *spreadConstraints {
		topologyName := selectedTopologyMapping[getTopologyFullKey(&constraint)]
		nextNode := node.children[topologyName]
		nextNode.selectedCount++
		node.maxChildrenSelectedCount = 0
		node.minChildrenSelectedCount = math.MaxInt32
		for _, n := range node.children {
			if n.selectedCount < node.minChildrenSelectedCount {
				node.minChildrenSelectedCount = n.selectedCount
			}
			if n.selectedCount > node.maxChildrenSelectedCount {
				node.maxChildrenSelectedCount = n.selectedCount
			}
		}
		node = nextNode
	}
}

// TopologyMapping is a map that contains all topology key to topology name mappings of a cluster.
type TopologyMapping map[topologyFullKey]string

type topologyNode struct {
	children                 map[string]*topologyNode // the topologies of the next layer
	selectedCount            int32                    // the number of clusters already selected in the topology
	minChildrenSelectedCount int32                    // the minimum number of clusters already selected for a topology in children
	maxChildrenSelectedCount int32                    // the maximum number of clusters already selected for a topology in children
}

func newTopologyNode() *topologyNode {
	return &topologyNode{
		children: make(map[string]*topologyNode),
	}
}

type topologyFullKey struct {
	topologyKey     string
	topologyKeyType clusterapiv1beta1.TopologyKeyType
}

func getTopologyFullKey(spreadConstraint *clusterapiv1beta1.SpreadConstraintsTerm) topologyFullKey {
	return topologyFullKey{
		topologyKey:     spreadConstraint.TopologyKey,
		topologyKeyType: spreadConstraint.TopologyKeyType,
	}
}
