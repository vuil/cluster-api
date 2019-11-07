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

package config

import (
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

// TODO:unit tests

// viperReader implements Reader using viper for reading from environment variablesClient
// and from a clusterctl config file.
type viperReader struct {
}

// newViperReader returns a viperReader.
func newViperReader() Reader {
	return &viperReader{}
}

// Init initialize the viperReader.
func (v *viperReader) Init(path string) error {
	if path != "" {
		// Use path file from the flag.
		viper.SetConfigFile(path)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			return errors.Wrap(err, "failed to get user's home directory")
		}

		// Configure for searching .clusterctl{.extension} in home directory and in current director
		viper.SetConfigName(".clusterctl")
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
	}

	// Configure for reading variablesClient variablesClient that match
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)

	viper.AutomaticEnv()

	// If a path file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		klog.V(1).Infof("Using %q configuration file", viper.ConfigFileUsed())
	}

	return nil
}

// Get return the environment variable with the given key. In case the variablesClient does not exists, an error is returned.
func (v *viperReader) GetString(key string) (string, error) {
	if viper.Get(key) == nil {
		return "", errors.Errorf("Failed to get value for variable %q. Please set the variable value using os env variables or using the .clusterctl config file", key)
	}
	return viper.GetString(key), nil
}

// UnmarshalKey takes a single key and unmarshal it into a Struct.
func (v *viperReader) UnmarshalKey(key string, rawval interface{}) error {
	return viper.UnmarshalKey(key, rawval)
}
