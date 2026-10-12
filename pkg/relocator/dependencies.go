package relocator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/provenance"
	sigsyaml "sigs.k8s.io/yaml"
)

// isDependencyVendored returns true if the dependency is already bundled in the
// charts/ directory of the chart, either unpacked or as a .tgz
func isDependencyVendored(chartsDir string, dep *chart.Dependency) bool {
	if info, err := os.Stat(filepath.Join(chartsDir, dep.Name)); err == nil && info.IsDir() {
		return true
	}
	if utils.FileExists(filepath.Join(chartsDir, dep.Name+".tgz")) {
		return true
	}
	matches, err := filepath.Glob(filepath.Join(chartsDir, dep.Name+"-*.tgz"))
	return err == nil && len(matches) > 0
}

// localizeDependencies clears the repository of the Chart.yaml dependencies that are already
// vendored under charts/, so Helm treats them as local subcharts and does not try to reach
// the original repository (i.e. on 'helm dependency build/update').
// If a Chart.lock file exists, it is updated accordingly, including its digest.
func localizeDependencies(c *cu.Chart) error {
	metadata := c.Metadata()
	if metadata == nil || len(metadata.Dependencies) == 0 {
		return nil
	}

	chartsDir := c.AbsFilePath("charts")
	localized := make(map[string]bool)
	for _, dep := range metadata.Dependencies {
		if dep.Repository != "" && isDependencyVendored(chartsDir, dep) {
			localized[dep.Name] = true
		}
	}
	if len(localized) == 0 {
		return nil
	}

	if err := clearChartYamlRepositories(c.AbsFilePath("Chart.yaml"), localized); err != nil {
		return err
	}
	for _, dep := range metadata.Dependencies {
		if localized[dep.Name] {
			dep.Repository = ""
		}
	}

	lockFile := c.AbsFilePath("Chart.lock")
	if utils.FileExists(lockFile) {
		if err := localizeChartLock(lockFile, metadata.Dependencies, localized); err != nil {
			return err
		}
	}
	return nil
}

// clearChartYamlRepositories sets to empty the repository of the named dependencies,
// editing the YAML tree to preserve the rest of the file
func clearChartYamlRepositories(chartYaml string, names map[string]bool) error {
	data, err := os.ReadFile(chartYaml)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", chartYaml, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse %q: %w", chartYaml, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("unexpected format of %q", chartYaml)
	}

	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "dependencies" || root.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for _, dep := range root.Content[i+1].Content {
			if dep.Kind != yaml.MappingNode || !names[mappingValue(dep, "name")] {
				continue
			}
			for j := 0; j+1 < len(dep.Content); j += 2 {
				if dep.Content[j].Value == "repository" {
					dep.Content[j+1].SetString("")
				}
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("failed to format %q: %w", chartYaml, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to format %q: %w", chartYaml, err)
	}
	return utils.SafeWriteFile(chartYaml, buf.Bytes(), 0644)
}

func mappingValue(n *yaml.Node, key string) string {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1].Value
		}
	}
	return ""
}

// localizeChartLock clears the repository of the localized dependencies in the Chart.lock
// and recomputes its digest, so it stays in sync with the updated Chart.yaml
func localizeChartLock(lockFile string, deps []*chart.Dependency, localized map[string]bool) error {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", lockFile, err)
	}
	var lock chart.Lock
	if err := sigsyaml.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("failed to parse %q: %w", lockFile, err)
	}
	for _, dep := range lock.Dependencies {
		if localized[dep.Name] {
			dep.Repository = ""
		}
	}
	if lock.Digest, err = hashDependencies(deps, lock.Dependencies); err != nil {
		return fmt.Errorf("failed to compute digest of %q: %w", lockFile, err)
	}

	data, err = sigsyaml.Marshal(&lock)
	if err != nil {
		return fmt.Errorf("failed to format %q: %w", lockFile, err)
	}
	return utils.SafeWriteFile(lockFile, data, 0644)
}

// hashDependencies mimics the Helm digest used to detect Chart.lock files out of sync
// with Chart.yaml (helm.sh/helm/v3/internal/resolver.HashReq, which is not importable)
func hashDependencies(req, lock []*chart.Dependency) (string, error) {
	data, err := json.Marshal([2][]*chart.Dependency{req, lock})
	if err != nil {
		return "", err
	}
	s, err := provenance.Digest(bytes.NewBuffer(data))
	return "sha256:" + s, err
}
