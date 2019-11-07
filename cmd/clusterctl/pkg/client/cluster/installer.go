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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProviderInstallerService defined methods for supporting providers installation.
type ProviderInstallerService interface {
	Add(repository.Components, bool) error
	Install() ([]repository.Components, error)
}

// installerService implements ProviderInstallerService and takes care of:
// - queueing the providers to install, allowing validation before actually starting to install
// - installing providers components and creating provider metadata
// - detaching namespaces from providers in case a namespace hosts more than one provider
type installerService struct {
	k8sproxy           K8SProxy
	providerComponents ComponentsClient
	providerMetadata   MetadataClient

	installQueue []repository.Components
}

var _ ProviderInstallerService = &installerService{}

func (i *installerService) Add(components repository.Components, force bool) error {
	if err := i.providerMetadata.Validate(components.Metadata()); err != nil {
		if !force {
			return errors.Wrapf(err, "Installing provider %q can lead to a non functioning management cluster (you can use --force to ignore this error).", components.Name())
		}
		//TODO: log warning when installing with force
	}

	i.installQueue = append(i.installQueue, components)
	return nil
}

func (i *installerService) Install() ([]repository.Components, error) {
	var ret []repository.Components //nolint
	for _, components := range i.installQueue {
		klog.V(3).Infof("Installing provider %s/%s:%s", components.TargetNamespace(), components.Name(), components.Version())

		// create the provider
		err := i.providerComponents.Create(components)
		if err != nil {
			return nil, err
		}

		// create providers metadata
		err = i.providerMetadata.Create(components.Metadata())
		if err != nil {
			return nil, err
		}

		ret = append(ret, components)
	}

	// check for namespaces with more than one provider.
	// in this case we should detach(*) the namespace from any specific provider, so we avoid accidental errors
	// when doing delete --force namespace for one of the providers.
	// detach = remove the clusterctl.cluster.x-k8s.io/provider label

	//TODO: move to provider components (at least in part Detach Namespace)
	providersByNamespace := map[string]int32{}
	providers, err := i.providerMetadata.List()
	if err != nil {
		return nil, err
	}

	for _, p := range providers {
		providersByNamespace[p.Namespace]++
	}

	// for all the namespaces with more than one provider
	for namespace, count := range providersByNamespace {
		if count > 1 {
			// detach the namespace (if not already detached)
			c, err := i.k8sproxy.NewClient()
			if err != nil {
				return nil, err
			}

			n := &corev1.Namespace{}
			key := client.ObjectKey{
				Name: namespace,
			}
			if err = c.Get(ctx, key, n); err != nil {
				return nil, errors.Wrapf(err, "failed to get Namespace/%s", namespace)
			}

			if _, ok := n.Labels[v1alpha3.ClusterctlProviderLabelName]; ok {
				delete(n.Labels, v1alpha3.ClusterctlProviderLabelName)
				if err = c.Update(ctx, n); err != nil {
					return nil, errors.Wrapf(err, "failed to update Namespace/%s", namespace)
				}
			}
		}
	}

	return ret, nil
}

func newProviderInstaller(k8sproxy K8SProxy, providerMetadata MetadataClient, providerComponents ComponentsClient) *installerService {
	return &installerService{
		k8sproxy:           k8sproxy,
		providerMetadata:   providerMetadata,
		providerComponents: providerComponents,
	}
}
