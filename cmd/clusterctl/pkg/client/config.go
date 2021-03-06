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

func (c *clusterctlClient) GetProvidersConfig() ([]Provider, error) {
	r, err := c.configClient.Providers().List()
	if err != nil {
		return nil, err
	}

	// Provider is an alias for config.Provider; this makes the conversion
	rr := make([]Provider, len(r))
	for i, provider := range r {
		rr[i] = provider
	}

	return rr, nil
}

func (c *clusterctlClient) GetProviderComponents(provider, targetNameSpace, watchingNamespace string) (Components, error) {
	components, err := c.getComponentsByName(provider, targetNameSpace, watchingNamespace)
	if err != nil {
		return nil, err
	}

	return components, nil
}
