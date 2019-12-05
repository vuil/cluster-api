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
	"context"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

const (
	httpsScheme              = "https"
	githubDomain             = "github.com"
	gitHubTokeVariable       = "github-token"
	gitHubReleaseRepository  = "releases"
	gitHubTreeRepository     = "tree"
	gitHubLatestReleaseLabel = "latest"
)

// gitHubRepository provide support for providers hosted on GitHub.
//
// There are two variants of the GitHub repository, one reading from GitHub releases, the other reading from GitHub code tree.
//
// Repository using GitHub releases has full support for version, including also "latest" meta version, but no support
// for “raw” component YAML files because there is no support for nested folders inside GitHub release assets.
//
// Repository using GitHub code tree has full support for version (the concept of “version” is mapped on branches and tags),
// and also support for “raw” component YAML, even though this is not recommended due to performance reasons.
type gitHubRepository struct {
	providerConfig           config.Provider
	configVariablesClient    config.VariablesClient
	authenticatingHTTPClient *http.Client
	owner                    string
	repository               string //this is the GitHub repository
	repositoryType           string
	defaultVersion           string
	rootPath                 string
	componentsPath           string
	kustomizeDir             string
	injectClient             *github.Client
}

var _ Repository = &gitHubRepository{}

func (g *gitHubRepository) DefaultVersion() string {
	return g.defaultVersion
}

func (g *gitHubRepository) RootPath() string {
	return g.rootPath
}

func (g *gitHubRepository) ComponentsPath() string {
	return g.componentsPath
}

func (g *gitHubRepository) KustomizeDir() string {
	return g.kustomizeDir
}

func (g *gitHubRepository) GetFiles(version, path string) (map[string][]byte, error) {
	if g.repositoryType == gitHubReleaseRepository {
		release, err := g.getReleaseByTag(version)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get GitHub release %s", version)
		}

		// download files from the release
		files, err := g.downloadFilesFromRelease(release, path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to download files from GitHub release %s", version)
		}

		return files, nil
	}

	// otherwise we are reading files from the repository source tree

	// get the selected sha (if not already specified in the Path)
	sha := version
	if len(version) != 40 {
		sha2, err := g.getSHA(version)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get SHA for tag or branch %q", version)
		}
		sha = sha2
	}

	// download files from the source tree
	files, err := g.downloadFilesFromTree(sha, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download files from GitHub %q tree", version)
	}

	return files, nil
}

func newGitHubRepository(providerConfig config.Provider, configVariablesClient config.VariablesClient) (*gitHubRepository, error) {
	if configVariablesClient == nil {
		return nil, errors.New("invalid arguments: configVariablesClient can't be nil")
	}

	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Wrap(err, "invalid url")
	}

	// if the url is a github repository
	if !(rURL.Scheme == httpsScheme && rURL.Host == githubDomain) {
		return nil, errors.New("invalid url: a GitHub repository url should start with https://github.com")
	}

	// Checks if the repository Path is in the expected format:
	t := strings.Split(strings.TrimPrefix(rURL.Path, "/"), "/")
	if len(t) < 5 || !(t[2] == gitHubReleaseRepository || t[2] == gitHubTreeRepository) {
		return nil, errors.Errorf(
			"invalid url: a GitHub repository url should be in the form https://github.com/{owner}/{Path}/%s/{latest|version-tag}/{componentsClient.yaml} or https://github.com/{owner}/{Path}/%s/{branch|tag|sha}/{componentsClient-yaml-path}[#kustomize-dir]",
			gitHubReleaseRepository,
			gitHubTreeRepository,
		)
	}

	// extracts all the info from path
	owner := t[0]
	repository := t[1]
	repositoryType := t[2]
	defaultVersion := t[3]
	path := strings.Join(t[4:], "/")

	// use path's directory as a rootPath
	rootPath := filepath.Dir(path)
	// use the file name (if any) as componentsPath
	componentsPath := strings.TrimPrefix(strings.TrimPrefix(path, rootPath), "/")
	// use the url's fragment as a kustomizeDir
	kustomizeDir := rURL.Fragment

	repo := &gitHubRepository{
		providerConfig:        providerConfig,
		configVariablesClient: configVariablesClient,
		owner:                 owner,
		repository:            repository,
		repositoryType:        repositoryType,
		defaultVersion:        defaultVersion,
		rootPath:              rootPath,
		componentsPath:        componentsPath,
		kustomizeDir:          kustomizeDir,
	}

	token, err := configVariablesClient.Get(gitHubTokeVariable)
	if err != nil {
		klog.V(1).Infof("The %q configuration variable is missing. Failing back to unauthenticated requests that allows for up to 60 requests per hour.", gitHubTokeVariable)
	}
	if err == nil {
		repo.setAuthenticatingClient(token)
	}

	if repositoryType == gitHubReleaseRepository && defaultVersion == gitHubLatestReleaseLabel {
		repo.defaultVersion, err = repo.getLatestRelease()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get GitHub latest version")
		}
	}

	return repo, nil
}

func (g *gitHubRepository) getClient() *github.Client {
	if g.injectClient != nil {
		return g.injectClient
	}
	return github.NewClient(g.authenticatingHTTPClient)
}

func (g *gitHubRepository) setAuthenticatingClient(token string) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	g.authenticatingHTTPClient = oauth2.NewClient(ctx, ts)
}

// getLatestRelease return the latest release for a github repository, according to
// semantic version order of the release tag name.
func (g *gitHubRepository) getLatestRelease() (string, error) {
	ctx := context.Background()
	client := g.getClient()

	// get all the releases
	// NB. currently Github API does not support result ordering, so it not possible to limit results
	releases, _, err := client.Repositories.ListReleases(ctx, g.owner, g.repository, nil)
	if err != nil {
		return "", g.handleGithubErr(err, "failed to get the list of releases")
	}

	// search for the latest release according to semantic version order of the release tag name.
	// releases with tag name that are not semantic version number are ignored
	var latestTag string
	var latestReleaseVersion *version.Version
	for _, r := range releases {
		r := r // pin

		if r.TagName == nil {
			continue
		}

		tagName := *r.TagName
		sv, err := version.ParseSemantic(tagName)
		if err != nil {
			// discard releases with tags that are not a valid semantic versions (the user can point explicitly to such releases)
			continue
		}

		if sv.PreRelease() != "" || sv.BuildMetadata() != "" {
			// discard pre-releases or build releases (the user can point explicitly to such releases)
			continue
		}

		if latestReleaseVersion == nil || latestReleaseVersion.LessThan(sv) {
			latestTag = tagName
			latestReleaseVersion = sv
		}
	}

	if latestTag == "" {
		return "", errors.New("failed to find releases tagged with a valid semantic version number")
	}

	return latestTag, nil
}

// getReleaseByTag return the github repository release with a specific tag name.
func (g *gitHubRepository) getReleaseByTag(tag string) (*github.RepositoryRelease, error) {
	ctx := context.Background()
	client := g.getClient()

	release, _, err := client.Repositories.GetReleaseByTag(ctx, g.owner, g.repository, tag)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to read release %q", tag)
	}

	if release == nil {
		return nil, errors.Errorf("failed to get release %q", tag)
	}

	return release, nil
}

// downloadFilesFromRelease download a file from release.
func (g *gitHubRepository) downloadFilesFromRelease(release *github.RepositoryRelease, fileName string) (map[string][]byte, error) {
	ctx := context.Background()
	client := g.getClient()

	absoluteFileName := filepath.Join(g.rootPath, fileName)

	// search for the file into the release assets, retrieving the asset id
	var assetID *int64
	for _, a := range release.Assets {
		if a.Name != nil && *a.Name == absoluteFileName {
			assetID = a.ID
			break
		}
	}
	if assetID == nil {
		return nil, errors.Errorf("failed to get file %q from %q release", fileName, *release.TagName)
	}

	// download the asset Content
	rc, red, err := client.Repositories.DownloadReleaseAsset(ctx, g.owner, g.repository, *assetID)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to download file %q from %q release", *release.TagName, fileName)
	}

	// handle the case when it is returned a ReaderCloser object for a release asset
	if rc != nil {
		defer rc.Close()

		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read downloaded file %q from %q release", *release.TagName, fileName)
		}

		return map[string][]byte{fileName: content}, nil
	}

	// handle the case when it is returned a redirect link for a release asset
	resp, err := http.Get(red) //nolint
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download file %q from %q release via redirect location %q", *release.TagName, fileName, red)
	}
	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read downloaded file %q from %q release via redirect location %q", *release.TagName, fileName, red)
	}
	return map[string][]byte{fileName: content}, nil
}

// Utilities for discovery of Assets hosted as a github repository tree componentsClient

// getSHA return the SHA1 identifier for a github repository branch or a tag
func (g *gitHubRepository) getSHA(branchOrTag string) (string, error) {
	ctx := context.Background()
	client := g.getClient()

	// Search within branches; if found, return the SHA
	//TODO: check if it possible to change from list to get (same for tags)
	branches, _, err := client.Repositories.ListBranches(ctx, g.owner, g.repository, nil)
	if err != nil {
		return "", g.handleGithubErr(err, "failed to get the list of branches")
	}

	for _, b := range branches {
		if b.Name != nil && *b.Name == branchOrTag {
			return *b.Commit.SHA, nil
		}
	}

	// Search within tags; if found, return the SHA
	tags, _, err := client.Repositories.ListTags(ctx, g.owner, g.repository, nil)
	if err != nil {
		return "", g.handleGithubErr(err, "failed to get the list of tags")
	}

	for _, t := range tags {
		if t.Name != nil && *t.Name == branchOrTag {
			return *t.Commit.SHA, nil
		}
	}

	return "", errors.Errorf("%q does not match any existing branch or tag", branchOrTag)
}

// downloadFilesFromTree download Assets from a file/folder in a github repository tree.
// If the Path is a folder, also sub-folders are read, recursively.
func (g *gitHubRepository) downloadFilesFromTree(sha, path string) (map[string][]byte, error) {
	ctx := context.Background()
	client := g.getClient()

	absolutePath := filepath.Join(g.rootPath, path)

	// gets the file/folder from the github repository tree
	fileContent, dirContent, _, err := client.Repositories.GetContents(ctx, g.owner, g.repository, absolutePath, nil)
	if err != nil {
		return nil, g.handleGithubErr(err, "failed to get file %q for sha %q", absolutePath, sha)
	}

	// handles the case when the Path is a file
	assets := map[string][]byte{} //nolint
	if fileContent != nil {
		if fileContent.Encoding == nil || *fileContent.Encoding != "base64" {
			return nil, errors.Errorf("invalid encoding detected for file %q for sha %q. Only base 64 encoding supported", absolutePath, sha)
		}

		content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode file %q for sha %q", absolutePath, sha)
		}

		assets[path] = content
		return assets, nil
	}

	// handles the case when the Path is a directory reading Assets recursively
	for _, item := range dirContent {
		relativePath := strings.TrimPrefix(*item.Path, g.rootPath)
		relativePath = strings.TrimPrefix(relativePath, "/")

		itemAssets, err := g.downloadFilesFromTree(sha, relativePath)
		if err != nil {
			return nil, err
		}

		for path, content := range itemAssets {
			assets[path] = content
		}
	}

	return assets, nil
}

func (g *gitHubRepository) handleGithubErr(err error, message string, args ...interface{}) error {
	if _, ok := err.(*github.RateLimitError); ok {
		return errors.New("rate limit for github api has been reached. Please wait one hour or get a personal API tokens a assign it to the GITHUB_TOKEN environment variable")
	}
	return errors.Wrapf(err, message, args...)
}
