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

func (c *clusterctlClient) Pivot(fromKubeconfig, toKubeconfig string) error {

	fromCluster, err := c.clusterClientFactory(fromKubeconfig)
	if err != nil {
		return nil
	}

	if _, err := fromCluster.ProviderMetadata().EnsureMetadata(); err != nil {
		return err
	}

	toCluster, err := c.clusterClientFactory(toKubeconfig)
	if err != nil {
		return nil
	}

	if _, err := toCluster.ProviderMetadata().EnsureMetadata(); err != nil {
		return err
	}

	if err := fromCluster.ProviderMover().Pivot(toCluster); err != nil {
		return err
	}

	return nil
}
