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

package repository

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

const (
	variableName  = "FOO"
	variableValue = "foo"
)

var controllerYaml = []byte("apiVersion: apps/v1\n" +
	"kind: Deployment\n" +
	"metadata:\n" +
	"  name: my-controller\n" +
	"spec:\n" +
	"  template:\n" +
	"    spec:\n" +
	"      containers:\n" +
	"      - name: manager\n")

const namespaceName = "capa-system"

var namespaceYaml = []byte("apiVersion: v1\n" +
	"kind: Namespace\n" +
	"metadata:\n" +
	fmt.Sprintf("  name: %s", namespaceName))

var configMapYaml = []byte("apiVersion: v1\n" +
	"data:\n" +
	fmt.Sprintf("  variable: ${%s}\n", variableName) +
	"kind: ConfigMap\n" +
	"metadata:\n" +
	"  name: manager")

func Test_componentsClient_Get(t *testing.T) {
	g := NewWithT(t)

	p1 := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)

	configClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithVar(variableName, variableValue)))
	g.Expect(err).NotTo(HaveOccurred())

	type fields struct {
		provider   config.Provider
		repository Repository
	}
	type args struct {
		version           string
		targetNamespace   string
		watchingNamespace string
		skipVariables     bool
	}
	type want struct {
		provider          config.Provider
		version           string
		targetNamespace   string
		watchingNamespace string
		variables         []string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "successfully gets the components",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0",      // version detected
				targetNamespace:   namespaceName, // default targetNamespace detected
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "successfully gets the components even with SkipVariables defined",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
				skipVariables:     true,
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0",      // version detected
				targetNamespace:   namespaceName, // default targetNamespace detected
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "targetNamespace overrides default targetNamespace",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0", // version detected
				targetNamespace:   "ns2",    // targetNamespace overrides default targetNamespace
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "watchingNamespace overrides default watchingNamespace",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "ns2",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0",               // version detected
				targetNamespace:   namespaceName,          // default targetNamespace detected
				watchingNamespace: "ns2",                  // watchingNamespace overrides default watchingNamespace
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "Fails if components file does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0"),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
		{
			name: "Fails if default targetNamespace does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
		{
			name: "Pass if default targetNamespace does not exists but a target targetNamespace is set",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v1.0.0",
				targetNamespace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider:          p1,
				version:           "v1.0.0", // version detected
				targetNamespace:   "ns2",    // target targetNamespace applied
				watchingNamespace: "",
				variables:         []string{variableName}, // variable detected
			},
			wantErr: false,
		},
		{
			name: "Fails if requested version does not exists",
			fields: fields{
				provider: p1,
				repository: test.NewFakeRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0").
					WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(controllerYaml, configMapYaml)),
			},
			args: args{
				version:           "v2.0.0",
				targetNamespace:   "",
				watchingNamespace: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			options := ComponentsOptions{
				Version:           tt.args.version,
				TargetNamespace:   tt.args.targetNamespace,
				WatchingNamespace: tt.args.watchingNamespace,
				SkipVariables:     tt.args.skipVariables,
			}
			f := newComponentsClient(tt.fields.provider, tt.fields.repository, configClient)
			got, err := f.Get(options)
			if tt.wantErr {
				gs.Expect(err).To(HaveOccurred())
				return
			}
			gs.Expect(err).NotTo(HaveOccurred())

			gs.Expect(got.Name()).To(Equal(tt.want.provider.Name()))
			gs.Expect(got.Type()).To(Equal(tt.want.provider.Type()))
			gs.Expect(got.Version()).To(Equal(tt.want.version))
			gs.Expect(got.TargetNamespace()).To(Equal(tt.want.targetNamespace))
			gs.Expect(got.WatchingNamespace()).To(Equal(tt.want.watchingNamespace))
			gs.Expect(got.Variables()).To(Equal(tt.want.variables))

			yaml, err := got.Yaml()
			if err != nil {
				t.Errorf("got.Yaml() error = %v", err)
				return
			}

			if !tt.args.skipVariables && len(tt.want.variables) > 0 {
				gs.Expect(yaml).To(ContainSubstring(variableValue))
			}

			// Verify that when SkipVariables is set we have all the variables
			// in the template without the values processed.
			if tt.args.skipVariables {
				for _, v := range tt.want.variables {
					gs.Expect(yaml).To(ContainSubstring(v))
				}
			}

			for _, o := range got.InstanceObjs() {
				for _, v := range []string{clusterctlv1.ClusterctlLabelName, clusterv1.ProviderLabelName} {
					gs.Expect(o.GetLabels()).To(HaveKey(v))
				}
			}

			for _, o := range got.SharedObjs() {
				for _, v := range []string{clusterctlv1.ClusterctlLabelName, clusterv1.ProviderLabelName, clusterctlv1.ClusterctlResourceLifecyleLabelName} {
					gs.Expect(o.GetLabels()).To(HaveKey(v))
				}
			}
		})
	}
}

func Test_componentsClient_YamlProcessor(t *testing.T) {
	t.Run("Get returns error if yaml processor cannot get Variables", func(t *testing.T) {
		g := NewWithT(t)

		p := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)
		configClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithVar(variableName, variableValue)))
		g.Expect(err).NotTo(HaveOccurred())
		r := test.NewFakeRepository().
			WithPaths("root", "components.yaml").
			WithDefaultVersion("v1.0.0").
			WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml))

		f := newComponentsClient(p, r, configClient)
		//override the yaml processor to one that will return an error
		f.processor = NewFakeProcessor().WithGetVariablesErr(errors.New("cannot get vars"))

		options := ComponentsOptions{
			Version:           "v1.0.0",
			TargetNamespace:   "default",
			WatchingNamespace: "",
		}
		components, err := f.Get(options)
		g.Expect(err).To(HaveOccurred())
		g.Expect(components).To(BeNil())
	})

	t.Run("Get returns error if yaml processor cannot process the raw yaml", func(t *testing.T) {
		g := NewWithT(t)

		p := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)
		configClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithVar(variableName, variableValue)))
		g.Expect(err).NotTo(HaveOccurred())
		r := test.NewFakeRepository().
			WithPaths("root", "components.yaml").
			WithDefaultVersion("v1.0.0").
			WithFile("v1.0.0", "components.yaml", utilyaml.JoinYaml(namespaceYaml, controllerYaml, configMapYaml))

		f := newComponentsClient(p, r, configClient)
		//override the yaml processor to one that will return an error
		f.processor = NewFakeProcessor().WithProcessErr(errors.New("cannot process"))

		options := ComponentsOptions{
			Version:           "v1.0.0",
			TargetNamespace:   "default",
			WatchingNamespace: "",
		}

		components, err := f.Get(options)
		g.Expect(err).To(HaveOccurred())
		g.Expect(components).To(BeNil())
	})
}
