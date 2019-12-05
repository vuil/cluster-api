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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

const fileScheme = "file"

// filesystemRepository provide support for providers hosted on local file system.
//
// Repository using file system has limited support for version (the repository hosts only one version, that is defined
// in the version.txt file), and full support for “raw” component YAML
type filesystemRepository struct {
	providerConfig config.Provider
	defaultVersion string
	rootPath       string
	componentsPath string
	kustomizeDir   string
}

var _ Repository = &filesystemRepository{}

func (f *filesystemRepository) DefaultVersion() string {
	return f.defaultVersion
}

func (f *filesystemRepository) RootPath() string {
	return f.rootPath
}

func (f *filesystemRepository) ComponentsPath() string {
	return f.componentsPath
}

func (f *filesystemRepository) KustomizeDir() string {
	return f.kustomizeDir
}

func (f *filesystemRepository) GetFiles(version, path string) (map[string][]byte, error) {
	if version != "" {
		repoVersion, err := f.getVersion()
		if err != nil {
			return nil, err
		}

		if repoVersion != version {
			return nil, errors.Errorf("failed to read release %q", version)
		}
	}

	files, err := f.getFiles(path)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func newFilesystemRepositoryImpl(providerConfig config.Provider) (*filesystemRepository, error) {
	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Wrap(err, "invalid url")
	}

	// if the url is a local repository
	if !(rURL.Scheme == "" || rURL.Scheme == fileScheme) {
		return nil, errors.New("invalid url: filesystem repository url should start with file:// or empty schema")
	}

	// use path's directory as a rootPath
	rootPath := filepath.Dir(rURL.Path)
	// use the file name (if any) as componentsPath
	componentsPath := strings.TrimPrefix(strings.TrimPrefix(rURL.Path, rootPath), "/")
	// use the url's fragment as a kustomizeDir
	kustomizeDir := rURL.Fragment

	f := &filesystemRepository{
		providerConfig: providerConfig,
		defaultVersion: "NA",
		rootPath:       rootPath,
		componentsPath: componentsPath,
		kustomizeDir:   kustomizeDir,
	}

	repoVersion, err := f.getVersion()
	if err == nil {
		f.defaultVersion = repoVersion
	}

	return f, nil
}

func (f *filesystemRepository) getVersion() (string, error) {
	versionFiles, err := f.getFiles("version.txt")
	if err != nil {
		return "", errors.Wrapf(err, "failed to check repository version. Please add a version.txt file")
	}
	if len(versionFiles) != 1 {
		return "", errors.Wrapf(err, "failed to check repository version. Please add a version.txt file")
	}
	var currentVersion string
	for _, v := range versionFiles {
		currentVersion = string(v)
		break
	}
	return currentVersion, nil
}

// getFiles return the files available in the local repository at a given path;, path should be relative to the
// repository rootPath. If path is a folder, also sub-folders are read recursively.
func (f *filesystemRepository) getFiles(path string) (map[string][]byte, error) {
	path = filepath.Join(f.rootPath, path)

	// explore the local folder/dir collecting relative Path for each file
	var paths []string
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !(filepath.Base(path) == "version.txt" || filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			return nil
		}

		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "error reading local repository")
	}

	// read Content from each path and pack Path and Content into a File object
	files := map[string][]byte{} //nolint
	for _, path := range paths {
		klog.V(3).Infof("Reading %q", path)
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading from %q", path)
		}

		relativePath := strings.TrimPrefix(path, f.rootPath)
		relativePath = strings.TrimPrefix(relativePath, "/")
		files[relativePath] = content
	}

	return files, nil
}
