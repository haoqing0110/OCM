/*
Copyright 2023.

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
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	apisv1alpha1 "open-cluster-management.io/cluster-permission/apis/v1alpha1"
	versioned "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	internalinterfaces "open-cluster-management.io/cluster-permission/client/informers/externalversions/internalinterfaces"
	v1alpha1 "open-cluster-management.io/cluster-permission/client/listers/apis/v1alpha1"
)

// ClusterPermissionInformer provides access to a shared informer and lister for
// ClusterPermissions.
type ClusterPermissionInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ClusterPermissionLister
}

type clusterPermissionInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewClusterPermissionInformer constructs a new informer for ClusterPermission type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewClusterPermissionInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredClusterPermissionInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredClusterPermissionInformer constructs a new informer for ClusterPermission type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredClusterPermissionInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ApisV1alpha1().ClusterPermissions(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ApisV1alpha1().ClusterPermissions(namespace).Watch(context.TODO(), options)
			},
		},
		&apisv1alpha1.ClusterPermission{},
		resyncPeriod,
		indexers,
	)
}

func (f *clusterPermissionInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredClusterPermissionInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *clusterPermissionInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&apisv1alpha1.ClusterPermission{}, f.defaultInformer)
}

func (f *clusterPermissionInformer) Lister() v1alpha1.ClusterPermissionLister {
	return v1alpha1.NewClusterPermissionLister(f.Informer().GetIndexer())
}
