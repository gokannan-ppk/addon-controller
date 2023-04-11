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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	addoncontrollerv1alpha1 "github.com/gokannan-ppk/addon-controller/pkg/apis/addoncontroller/v1alpha1"
	versioned "github.com/gokannan-ppk/addon-controller/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/gokannan-ppk/addon-controller/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/gokannan-ppk/addon-controller/pkg/generated/listers/addoncontroller/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// AddonInformer provides access to a shared informer and lister for
// Addons.
type AddonInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.AddonLister
}

type addonInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewAddonInformer constructs a new informer for Addon type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewAddonInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredAddonInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredAddonInformer constructs a new informer for Addon type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredAddonInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.AddoncontrollerV1alpha1().Addons(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.AddoncontrollerV1alpha1().Addons(namespace).Watch(context.TODO(), options)
			},
		},
		&addoncontrollerv1alpha1.Addon{},
		resyncPeriod,
		indexers,
	)
}

func (f *addonInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredAddonInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *addonInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&addoncontrollerv1alpha1.Addon{}, f.defaultInformer)
}

func (f *addonInformer) Lister() v1alpha1.AddonLister {
	return v1alpha1.NewAddonLister(f.Informer().GetIndexer())
}
