/*
Copyright 2020 The Kubernetes Authors.
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
package yamlprocessor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
)

// SimpleProcessor is a yaml processor that does simple variable
// substitution. The variables are defined in the following format
// ${variable_name}
type SimpleProcessor struct{}

func NewSimpleProcessor() *SimpleProcessor {
	return &SimpleProcessor{}
}

func (tp *SimpleProcessor) ArtifactName(version, flavor string) string {
	// building template name according with the naming convention
	name := "cluster-template"
	if flavor != "" {
		name = fmt.Sprintf("%s-%s", name, flavor)
	}
	name = fmt.Sprintf("%s.yaml", name)

	return name
}

func (tp *SimpleProcessor) GetVariables(rawArtifact []byte) ([]string, error) {
	return inspectVariables(rawArtifact), nil
}

func (tp *SimpleProcessor) Process(rawArtifact []byte, variablesClient config.VariablesClient) ([]byte, error) {
	variables := inspectVariables(rawArtifact)

	tmp := string(rawArtifact)
	var err error
	var missingVariables []string
	for _, key := range variables {
		val, err := variablesClient.Get(key)
		if err != nil {
			missingVariables = append(missingVariables, key)
			continue
		}
		exp := regexp.MustCompile(`\$\{\s*` + regexp.QuoteMeta(key) + `\s*\}`)
		tmp = exp.ReplaceAllLiteralString(tmp, val)
	}
	if len(missingVariables) > 0 {
		err = errors.Errorf("value for variables [%s] is not set. Please set the value using os environment variables or the clusterctl config file", strings.Join(missingVariables, ", "))
	}

	return []byte(tmp), err
}

// variableRegEx defines the regexp used for searching variables inside a YAML
var variableRegEx = regexp.MustCompile(`\${\s*([A-Z0-9_]+)\s*}`)

func inspectVariables(data []byte) []string {
	variables := sets.NewString()
	match := variableRegEx.FindAllStringSubmatch(string(data), -1)

	for _, m := range match {
		submatch := m[1]
		if !variables.Has(submatch) {
			variables.Insert(submatch)
		}
	}

	ret := variables.List()
	sort.Strings(ret)
	return ret
}
