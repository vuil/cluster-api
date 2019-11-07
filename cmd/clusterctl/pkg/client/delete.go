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
	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

func (c *clusterctlClient) Delete(kubeconfig string, forceDeleteNamespace, forceDeleteCRD bool, providers ...string) error {
	cluster, err := c.clusterClientFactory(kubeconfig)
	if err != nil {
		return err
	}

	if _, err := cluster.ProviderMetadata().EnsureMetadata(); err != nil {
		return err
	}

	// Get the list of installed providers
	installedProviders, err := cluster.ProviderMetadata().List()
	if err != nil {
		return err
	}

	// If the list of providers to delete is empty, delete all the providers
	if len(providers) == 0 {
		for _, provider := range installedProviders {
			if err := cluster.ProviderComponents().Delete(provider, forceDeleteNamespace, forceDeleteCRD); err != nil {
				return err
			}
		}
	}

	// Otherwise we are deleting only specific providers

	// Prepare the list of providers to delete
	var providersToDelete []clusterctlv1.Provider
	for _, provider := range providers {
		// parse the UX provider name
		namespace, name, _, err := parseProviderName(provider)
		if err != nil {
			return err
		}

		// if provider namespace, get it from cluster
		if namespace == "" {
			namespace, err = cluster.ProviderMetadata().GetDefaultNamespace(name)
			if err != nil {
				return err
			}

			// if there are more instance of a providers, it is not possible to get a default namespace for the provider,
			// so we should return and ask for it.
			if namespace == "" {
				return errors.Errorf("Unable to find default namespace for %q provider. Please specify the provider's namespace", name)
			}
		}

		// check the provider actually match one of the installed providers
		found := false
		for _, ip := range installedProviders {
			if ip.Name != name {
				continue
			}

			if ip.Name == name && ip.Namespace == namespace {
				found = true
				providersToDelete = append(providersToDelete, ip)
				break
			}
		}

		if !found {
			return errors.Errorf("Failed to find provider %q", provider)
		}
	}

	// Delete the providers
	for _, provider := range providersToDelete {
		if err := cluster.ProviderComponents().Delete(provider, forceDeleteNamespace, forceDeleteCRD); err != nil {
			return err
		}
	}

	return nil
}
