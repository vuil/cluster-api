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

package client

import (
	"unsafe"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

func (c *clusterctlClient) Init(kubeconfig string, coreProvider string, bootstrapProviders, infrastructureProviders []string, targetNameSpace, watchingNamespace string, force bool) ([]Components, bool, error) {
	cluster, err := c.clusterClientFactory(kubeconfig)
	if err != nil {
		return nil, false, err
	}

	hasCRD, err := cluster.ProviderMetadata().EnsureMetadata()
	if err != nil {
		return nil, false, err
	}

	if !hasCRD {
		if coreProvider == "" {
			coreProvider = config.CAPI
		}

		//TODO: check at least an infra provider is specified
	}

	installer := cluster.ProviderInstaller()

	if coreProvider != "" {
		if err := c.addToInstaller(installer, clusterctlv1.CoreProviderType, targetNameSpace, watchingNamespace, force, coreProvider); err != nil {
			return nil, false, err
		}
	}

	if err := c.addToInstaller(installer, clusterctlv1.BootstrapProviderType, targetNameSpace, watchingNamespace, force, bootstrapProviders...); err != nil {
		return nil, false, err
	}

	if err := c.addToInstaller(installer, clusterctlv1.InfrastructureProviderType, targetNameSpace, watchingNamespace, force, infrastructureProviders...); err != nil {
		return nil, false, err
	}

	r, err := installer.Install()
	if err != nil {
		return nil, false, err
	}

	// Components is an alias for repository.Components; this makes low level conversion
	rr := *(*[]Components)(unsafe.Pointer(&r))

	return rr, !hasCRD, nil
}

func (c *clusterctlClient) addToInstaller(installer cluster.ProviderInstallerService, ttype clusterctlv1.ProviderType, targetNameSpace string, watchingNamespace string, force bool, providers ...string) error {
	for _, provider := range providers {
		components, err := c.getComponentsByName(provider, targetNameSpace, watchingNamespace)
		if err != nil {
			return err
		}

		if components.Type() != ttype {
			return errors.Errorf("can't use %q provider as an %q, it is a %q", provider, ttype, components.Type())
		}

		if err := installer.Add(components, force); err != nil {
			return err
		}
	}
	return nil
}
