/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"k8s.io/klog"
)

// ProviderMoverService defined methods for supporting providers pivoting.
type ProviderMoverService interface {
	Pivot(toCluster Client) error
}

// moverService implements ProviderMoverService and takes care of
// pivoting provider components, provider objects, providers metadata in the right order
type moverService struct {
	providerMetadata   MetadataClient
	providerComponents ComponentsClient
	providerObjects    ObjectsClient
}

var _ ProviderMoverService = &moverService{}

func (p *moverService) Pivot(toCluster Client) error {
	// prepare provider components for move into to cluster

	providers, err := p.providerMetadata.List()
	if err != nil {
		return err
	}

	klog.V(2).Infof("Installing providers in the target cluster")

	for _, provider := range providers {
		//TODO: keep this check? (skip copy if provider already exists)
		installedProvider, _ := toCluster.ProviderMetadata().Get(provider.Namespace, provider.Name)
		if installedProvider != nil && installedProvider.Version == provider.Version && installedProvider.WatchedNamespace == provider.WatchedNamespace {
			continue
		}

		if err := toCluster.ProviderMetadata().Validate(provider); err != nil {
			return err
		}

		if err := p.providerComponents.Pivot(toCluster); err != nil {
			return err
		}
	}

	// scale down provider components
	klog.V(2).Infof("Scaling down controllers in the source cluster")
	for _, provider := range providers {
		if err := p.providerComponents.ScaleDownControllers(provider); err != nil {
			return err
		}
	}

	// move objects
	klog.V(2).Infof("Moving objects to the target cluster")
	if err := p.providerObjects.Pivot(toCluster.ProviderObjects()); err != nil {
		return err
	}

	// delete provider components from
	klog.V(2).Infof("Deleting providers in the source cluster")
	forceDeleteNamespace := false //TODO: input parameter?
	for i, provider := range providers {
		forceDeleteCRD := true
		for _, pnext := range providers[i:] {
			if provider.Name == pnext.Name {
				forceDeleteCRD = false
			}
		}

		err := p.providerComponents.Delete(provider, forceDeleteNamespace, forceDeleteCRD)
		if err != nil {
			return err
		}
	}
	return nil
}

func newProviderMover(providerMetadata MetadataClient, providerComponents ComponentsClient, providerObjects ObjectsClient) *moverService {
	return &moverService{
		providerMetadata:   providerMetadata,
		providerComponents: providerComponents,
		providerObjects:    providerObjects,
	}
}
