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
	v1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// MultiKueueClusterLister helps list MultiKueueClusters.
// All objects returned here must be treated as read-only.
type MultiKueueClusterLister interface {
	// List lists all MultiKueueClusters in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.MultiKueueCluster, err error)
	// Get retrieves the MultiKueueCluster from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta1.MultiKueueCluster, error)
	MultiKueueClusterListerExpansion
}

// multiKueueClusterLister implements the MultiKueueClusterLister interface.
type multiKueueClusterLister struct {
	listers.ResourceIndexer[*v1beta1.MultiKueueCluster]
}

// NewMultiKueueClusterLister returns a new MultiKueueClusterLister.
func NewMultiKueueClusterLister(indexer cache.Indexer) MultiKueueClusterLister {
	return &multiKueueClusterLister{listers.New[*v1beta1.MultiKueueCluster](indexer, v1beta1.Resource("multikueuecluster"))}
}
