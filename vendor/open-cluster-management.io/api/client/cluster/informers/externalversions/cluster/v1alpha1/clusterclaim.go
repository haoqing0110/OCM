// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	versioned "open-cluster-management.io/api/client/cluster/clientset/versioned"
	internalinterfaces "open-cluster-management.io/api/client/cluster/informers/externalversions/internalinterfaces"
	v1alpha1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1alpha1"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
)

// ClusterClaimInformer provides access to a shared informer and lister for
// ClusterClaims.
type ClusterClaimInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ClusterClaimLister
}

type clusterClaimInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewClusterClaimInformer constructs a new informer for ClusterClaim type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewClusterClaimInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredClusterClaimInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredClusterClaimInformer constructs a new informer for ClusterClaim type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredClusterClaimInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ClusterV1alpha1().ClusterClaims().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ClusterV1alpha1().ClusterClaims().Watch(context.TODO(), options)
			},
		},
		&clusterv1alpha1.ClusterClaim{},
		resyncPeriod,
		indexers,
	)
}

func (f *clusterClaimInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredClusterClaimInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *clusterClaimInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&clusterv1alpha1.ClusterClaim{}, f.defaultInformer)
}

func (f *clusterClaimInformer) Lister() v1alpha1.ClusterClaimLister {
	return v1alpha1.NewClusterClaimLister(f.Informer().GetIndexer())
}