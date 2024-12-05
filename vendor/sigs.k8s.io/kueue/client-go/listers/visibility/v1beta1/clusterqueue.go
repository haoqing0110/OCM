/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
	v1beta1 "sigs.k8s.io/kueue/apis/visibility/v1beta1"
)

// ClusterQueueLister helps list ClusterQueues.
// All objects returned here must be treated as read-only.
type ClusterQueueLister interface {
	// List lists all ClusterQueues in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.ClusterQueue, err error)
	// Get retrieves the ClusterQueue from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta1.ClusterQueue, error)
	ClusterQueueListerExpansion
}

// clusterQueueLister implements the ClusterQueueLister interface.
type clusterQueueLister struct {
	listers.ResourceIndexer[*v1beta1.ClusterQueue]
}

// NewClusterQueueLister returns a new ClusterQueueLister.
func NewClusterQueueLister(indexer cache.Indexer) ClusterQueueLister {
	return &clusterQueueLister{listers.New[*v1beta1.ClusterQueue](indexer, v1beta1.Resource("clusterqueue"))}
}
