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

package cmd

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

type pivotOptions struct {
	fromKubeconfig string
	toKubeconfig   string
}

var po = &pivotOptions{}

var pivotCmd = &cobra.Command{
	Use:   "pivot",
	Short: "Pivot a management cluster into another management cluster",
	Long: LongDesc(`
		Pivot the content of a management cluster, provider components and objects, 
		into another management cluster.`),

	Example: Examples(`
		# Pivot current management cluster into a target cluster.
		clusterctl pivot --to=target-kubeconfig.yaml`),

	RunE: func(cmd *cobra.Command, args []string) error {
		if po.toKubeconfig == "" {
			return errors.New("please specify a target cluster using the --to flag")
		}

		return runPivot()
	},
}

func init() {
	pivotCmd.Flags().StringVarP(&po.fromKubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the originating management cluster. If empty, default rules for kubeconfig discovery will be used")
	pivotCmd.Flags().StringVarP(&po.toKubeconfig, "to", "", "", "Path to the kubeconfig file to use for accessing the target management cluster")

	RootCmd.AddCommand(pivotCmd)
}

func runPivot() error {
	c, err := client.New(cfgFile, client.Options{})
	if err != nil {
		return err
	}

	if err := c.Pivot(po.fromKubeconfig, po.toKubeconfig); err != nil {
		return err
	}

	return nil
}
