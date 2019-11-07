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
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

//TODO: test newFilesystemRepositoryImpl

func Test_filesystemRepository_getFiles(t *testing.T) {
	dir, err := ioutil.TempDir("", "clusterctl")
	if err != nil {
		t.Fatalf("ioutil.TempDir() error = %v", err)
	}
	defer os.RemoveAll(dir)

	if err := ioutil.WriteFile(filepath.Join(dir, "version.txt"), []byte("v1.0.0"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "config/crd"), 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "config/default"), 0700); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "config/crd/kustomization.yaml"), []byte("content"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "config/crd/components.yaml"), []byte("content"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "config/default/kustomization.yaml"), []byte("content"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

	providerConfig := config.NewProvider("test", dir+"/", clusterctlv1.CoreProviderType) //tree/master/path not relevant for the test

	type args struct {
		version string
		path    string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string][]byte
		wantErr bool
	}{
		{
			name: "Return file",
			args: args{
				version: "v1.0.0",
				path:    "config/crd/components.yaml",
			},
			want: map[string][]byte{
				"config/crd/components.yaml": []byte("content"),
			},
			wantErr: false,
		},
		{
			name: "Return folder",
			args: args{
				version: "v1.0.0",
				path:    "config/crd/",
			},
			want: map[string][]byte{
				"config/crd/kustomization.yaml": []byte("content"),
				"config/crd/components.yaml":    []byte("content"),
			},
			wantErr: false,
		},
		{
			name: " folder with nested folder",
			args: args{
				version: "v1.0.0",
				path:    "config/",
			},
			want: map[string][]byte{
				"config/crd/kustomization.yaml":     []byte("content"),
				"config/crd/components.yaml":        []byte("content"),
				"config/default/kustomization.yaml": []byte("content"),
			},
			wantErr: false,
		},
		{
			name: "Fails if file does not exists",
			args: args{
				version: "v1.0.0",
				path:    "foo.yaml",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails if wrong version",
			args: args{
				version: "v2.0.0",
				path:    "config/crd/components.yaml",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := newFilesystemRepositoryImpl(providerConfig)
			if err != nil {
				t.Fatalf("newFilesystemRepositoryImpl() error = %v", err)
			}

			got, err := f.GetFiles(tt.args.version, tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFiles() got = %v, want %v", got, tt.want)
			}
		})
	}
}
