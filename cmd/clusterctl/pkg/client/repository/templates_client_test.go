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
	"bytes"
	"fmt"
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

var templateMapYaml = []byte("apiVersion: v1\n" +
	"data:\n" +
	fmt.Sprintf("  variable: ${%s}\n", variableName) +
	"  template: {{ .ClusterName }}\n" +
	"kind: ConfigMap\n" +
	"metadata:\n" +
	"  name: manager")

//noinspection ALL
func Test_templates_Get(t *testing.T) {
	p1 := config.NewProvider("p1", "", clusterctlv1.BootstrapProviderType)

	type fields struct {
		version               string
		provider              config.Provider
		repository            Repository
		configVariablesClient config.VariablesClient
	}
	type args struct {
		flavor          string
		bootstrap       string
		templateOptions TemplateOptions
	}
	type want struct {
		provider        config.Provider
		version         string
		flavor          string
		bootstrap       string
		path            string
		variables       []string
		targetNamespace string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "pass if default template exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: test.NewFakeRepository().
					WithDefaultVersion("v1.0").
					WithPaths("root", "", "").
					WithFile("v1.0", "config-kubeadm.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				flavor:    "",
				bootstrap: "kubeadm",
				templateOptions: TemplateOptions{
					ClusterName: "test-cluster",
					Namespace:   "ns1",
				},
			},
			want: want{
				provider:        p1,
				version:         "v1.0",
				flavor:          "",
				bootstrap:       "kubeadm",
				path:            "config-kubeadm.yaml",
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
		{
			name: "pass if template for a flavor exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: test.NewFakeRepository().
					WithDefaultVersion("v1.0").
					WithPaths("root", "", "").
					WithFile("v1.0", "config-prod-kubeadm.yaml", templateMapYaml),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				flavor:    "prod",
				bootstrap: "kubeadm",
				templateOptions: TemplateOptions{
					ClusterName: "test-cluster",
					Namespace:   "ns1",
				},
			},
			want: want{
				provider:        p1,
				version:         "v1.0",
				flavor:          "prod",
				bootstrap:       "kubeadm",
				path:            "config-prod-kubeadm.yaml",
				variables:       []string{variableName},
				targetNamespace: "ns1",
			},
			wantErr: false,
		},
		{
			name: "fails if template does not exists",
			fields: fields{
				version:  "v1.0",
				provider: p1,
				repository: test.NewFakeRepository().
					WithDefaultVersion("v1.0").
					WithPaths("root", "", ""),
				configVariablesClient: test.NewFakeVariableClient().WithVar(variableName, variableValue),
			},
			args: args{
				flavor:    "",
				bootstrap: "kubeadm",
				templateOptions: TemplateOptions{
					ClusterName: "test-cluster",
					Namespace:   "ns1",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTemplatesClient(tt.fields.provider, tt.fields.version, tt.fields.repository, tt.fields.configVariablesClient)
			got, err := f.Get(tt.args.flavor, tt.args.bootstrap, tt.args.templateOptions)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.provider.Name() {
				t.Errorf("Get().Name() got = %v, want = %v ", got.Name(), tt.want.provider.Name())
			}

			if got.Type() != tt.want.provider.Type() {
				t.Errorf("Get().Type()  got = %v, want = %v ", got.Type(), tt.want.provider.Type())
			}

			if got.Version() != tt.want.version {
				t.Errorf("Get().Version() got = %v, want = %v ", got.Version(), tt.want.version)
			}

			if got.Bootstrap() != tt.want.bootstrap {
				t.Errorf("Get().Bootstrap() got = %v, want = %v ", got.Bootstrap(), tt.want.bootstrap)
			}

			if got.Path() != tt.want.path {
				t.Errorf("Get().Path() got = %v, want = %v ", got.Path(), tt.want.path)
			}

			if !reflect.DeepEqual(got.Variables(), tt.want.variables) {
				t.Errorf("Get().Variables() got = %v, want = %v ", got.Variables(), tt.want.variables)
			}

			if !reflect.DeepEqual(got.TargetNamespace(), tt.want.targetNamespace) {
				t.Errorf("Get().TargetNamespace() got = %v, want = %v ", got.TargetNamespace(), tt.want.targetNamespace)
			}

			// check variable replaced in components
			if !bytes.Contains(got.Yaml(), []byte(fmt.Sprintf("variable: %s", variableValue))) {
				t.Error("Get().Yaml without variable substitution")
			}

			// check go templated run
			if !bytes.Contains(got.Yaml(), []byte(fmt.Sprintf("template: %s", tt.args.templateOptions.ClusterName))) {
				t.Error("Get().Yaml without go template execution")
			}
			//TODO: check targetNamespace in objects
		})
	}
}
