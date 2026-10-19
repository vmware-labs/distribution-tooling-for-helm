package relocator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	cu "github.com/vmware-labs/distribution-tooling-for-helm/pkg/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/provenance"
	sigsyaml "sigs.k8s.io/yaml"
)

// isDependencyVendored returns true if the dependency is bundled, unpacked, in the charts/
// directory of the chart and its version satisfies the version constraint of the dependency.
//
// Both conditions are required for Helm to treat the dependency as a local subchart: with an
// empty repository, 'helm dependency build/update' loads charts/<name> as a directory and
// validates its version against the constraint.
func isDependencyVendored(chartsDir string, dep *chart.Dependency) bool {
	depDir := filepath.Join(chartsDir, dep.Name)
	if info, err := os.Stat(depDir); err != nil || !info.IsDir() {
		return false
	}
	depMetadata, err := chartutil.LoadChartfile(filepath.Join(depDir, "Chart.yaml"))
	if err != nil {
		return false
	}

	constraint := dep.Version
	if constraint == "" {
		constraint = "*"
	}
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false
	}
	v, err := semver.NewVersion(depMetadata.Version)
	if err != nil {
		return false
	}
	return c.Check(v)
}

// localizeDependencies clears the repository of the Chart.yaml dependencies that are already
// vendored under charts/, so Helm treats them as local subcharts and does not try to reach
// the original repository (i.e. on 'helm dependency build/update').
// If a Chart.lock file exists, it is updated accordingly, including its digest.
//
// Dependencies bundled as .tgz archives are unpacked first, like the recursive relocation does,
// as Helm requires an unpacked charts/<name> directory when the repository is empty.
//
// Both Chart.yaml and Chart.lock are computed before writing anything, so a failure does not
// leave them out of sync.
func localizeDependencies(c *cu.Chart) error {
	metadata := c.Metadata()
	if metadata == nil {
		return nil
	}
	hasRemoteDeps := false
	for _, dep := range metadata.Dependencies {
		if dep.Repository != "" {
			hasRemoteDeps = true
			break
		}
	}
	if !hasRemoteDeps {
		return nil
	}

	if _, err := c.Dependencies(); err != nil {
		return fmt.Errorf("failed to unpack chart dependencies: %w", err)
	}

	chartsDir := c.AbsFilePath("charts")
	// Chart.yaml dependency index -> localized
	localized := make(map[int]bool)
	localizedNames := make(map[string]*chart.Dependency)
	for i, dep := range metadata.Dependencies {
		if dep.Repository != "" && isDependencyVendored(chartsDir, dep) {
			localized[i] = true
			localizedNames[dep.Name] = dep
		}
	}
	if len(localized) == 0 {
		return nil
	}

	chartYamlFile := c.AbsFilePath("Chart.yaml")
	chartYamlData, err := os.ReadFile(chartYamlFile)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", chartYamlFile, err)
	}
	newChartYaml, err := clearChartYamlRepositories(chartYamlData, localized)
	if err != nil {
		return fmt.Errorf("failed to update %q: %w", chartYamlFile, err)
	}

	// Dependencies as they will be after clearing the repositories
	newDeps := make([]*chart.Dependency, len(metadata.Dependencies))
	for i, dep := range metadata.Dependencies {
		newDep := *dep
		if localized[i] {
			newDep.Repository = ""
		}
		newDeps[i] = &newDep
	}

	lockFile := c.AbsFilePath("Chart.lock")
	var newLock []byte
	if utils.FileExists(lockFile) {
		if newLock, err = localizeChartLock(lockFile, newDeps, localizedNames); err != nil {
			return err
		}
	}

	if err = utils.SafeWriteFile(chartYamlFile, newChartYaml, 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", chartYamlFile, err)
	}
	if newLock != nil {
		if err = utils.SafeWriteFile(lockFile, newLock, 0644); err != nil {
			// Do not leave Chart.yaml and Chart.lock out of sync
			if rErr := utils.SafeWriteFile(chartYamlFile, chartYamlData, 0644); rErr != nil {
				return fmt.Errorf("failed to write %q: %w (and failed to restore %q: %v)", lockFile, err, chartYamlFile, rErr)
			}
			return fmt.Errorf("failed to write %q: %w", lockFile, err)
		}
	}

	for i, dep := range metadata.Dependencies {
		if localized[i] {
			dep.Repository = ""
		}
	}
	return nil
}

// clearChartYamlRepositories sets to empty the repository of the Chart.yaml dependencies
// at the specified indexes
func clearChartYamlRepositories(chartYaml []byte, indexes map[int]bool) ([]byte, error) {
	values := make(map[string]string, len(indexes))
	for i := range indexes {
		values[fmt.Sprintf("$.dependencies[%d].repository", i)] = ""
	}
	return utils.YamlSet(chartYaml, values)
}

// localizeChartLock returns the contents of the Chart.lock with the repository of the localized
// dependencies cleared and the digest recomputed, so it stays in sync with the updated Chart.yaml.
//
// For dependencies without a repository, Helm records in the lock the version constraint of the
// Chart.yaml instead of a resolved version, so we do the same to match what Helm would generate.
func localizeChartLock(lockFile string, deps []*chart.Dependency, localized map[string]*chart.Dependency) ([]byte, error) {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", lockFile, err)
	}
	var lock chart.Lock
	if err = sigsyaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", lockFile, err)
	}
	for _, lockDep := range lock.Dependencies {
		if dep, ok := localized[lockDep.Name]; ok {
			lockDep.Repository = ""
			lockDep.Version = dep.Version
		}
	}
	if lock.Digest, err = hashDependencies(deps, lock.Dependencies); err != nil {
		return nil, fmt.Errorf("failed to compute digest of %q: %w", lockFile, err)
	}

	data, err = sigsyaml.Marshal(&lock)
	if err != nil {
		return nil, fmt.Errorf("failed to format %q: %w", lockFile, err)
	}
	return data, nil
}

// hashDependencies mimics the Helm digest used to detect Chart.lock files out of sync
// with Chart.yaml (helm.sh/helm/v3/internal/resolver.HashReq, which is not importable).
// Tests validate it against Helm itself.
func hashDependencies(req, lock []*chart.Dependency) (string, error) {
	data, err := json.Marshal([2][]*chart.Dependency{req, lock})
	if err != nil {
		return "", err
	}
	s, err := provenance.Digest(bytes.NewBuffer(data))
	return "sha256:" + s, err
}
