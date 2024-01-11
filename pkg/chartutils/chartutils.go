package chartutils

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"gopkg.in/yaml.v3"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// ErrNoImagesToAnnotate is returned when the chart can't be annotated because
// there are no container images
var ErrNoImagesToAnnotate = errors.New("no container images to annotate found")

// AnnotateChart parses the values.yaml file in the chart specified by chartPath and
// annotates the Chart with the list of found images
func AnnotateChart(chartPath string, opts ...Option) error {
	n, err := annotateChart(chartPath, opts...)
	if n == 0 && err == nil {
		return ErrNoImagesToAnnotate
	}

	return err
}

func annotateChart(chartPath string, opts ...Option) (int, error) {
	cfg := NewConfiguration(opts...)
	chart, err := loader.Load(chartPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load Helm chart: %v", err)
	}

	chartRoot, err := GetChartRoot(chartPath)
	if err != nil {
		return 0, fmt.Errorf("cannot determine Helm chart root: %v", err)
	}

	res, err := FindImageElementsInValuesFile(chartPath)
	if err != nil {
		return 0, fmt.Errorf("failed to find image elements: %v", err)
	}

	nImages := len(res)
	// Make sure order is always the same
	sort.Sort(res)

	chartFile := filepath.Join(chartRoot, "Chart.yaml")

	if err := writeAnnotationsToChart(res, chartFile, cfg); err != nil {
		return 0, fmt.Errorf("failed to serialize annotations: %v", err)
	}
	var allErrors error
	for _, dep := range chart.Dependencies() {
		subChart := filepath.Join(chartRoot, "charts", dep.Name())
		n, err := annotateChart(subChart, opts...)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("failed to annotate sub-chart %q: %v", dep.ChartFullPath(), err))
		}
		nImages += n
	}
	return nImages, allErrors
}

// GetChartRoot returns the chart root directory to the chart provided (which may point to its Chart.yaml file)
func GetChartRoot(chartPath string) (string, error) {
	fi, err := os.Stat(chartPath)
	if err != nil {
		return "", fmt.Errorf("cannot access path %q: %v", chartPath, err)
	}
	// we either got the path to chart dir, or to the chart yaml
	if fi.IsDir() {
		return filepath.Abs(chartPath)
	}
	return filepath.Abs(filepath.Dir(chartPath))
}

func writeAnnotationsToChart(set ValuesImageElementList, chartFile string, cfg *Configuration) error {
	// Nothing to write
	if len(set) == 0 {
		return nil
	}
	imagesAnnotation, err := set.ToAnnotation()
	if err != nil {
		return fmt.Errorf("failed to create annotation text: %v", err)
	}

	type YAMLData struct {
		Annotations map[string]interface{} `yaml:"annotations"`

		Rest map[string]interface{} `yaml:",inline"`
	}

	chartData, err := os.ReadFile(chartFile)
	if err != nil {
		log.Fatalf("Failed to unmarshal YAML: %v", err)
	}
	var data YAMLData

	// Unmarshal the YAML into the struct
	err = yaml.Unmarshal(chartData, &data)
	if err != nil {
		log.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// The map is nil if the chart does not contain annotations
	if data.Annotations == nil {
		data.Annotations = make(map[string]interface{})
	}
	// Do any necessary modifications to the annotations field
	data.Annotations[cfg.AnnotationsKey] = string(imagesAnnotation)
	// Marshal the struct back into YAML
	modifiedYAML, err := yaml.Marshal(&data)
	if err != nil {
		log.Fatalf("Failed to marshal YAML: %v", err)
	}
	return utils.SafeWriteFile(chartFile, modifiedYAML, 0600)
}

func getChartFile(c *chart.Chart, name string) *chart.File {
	for _, f := range c.Raw {
		if f.Name == name {
			return f

		}
	}
	return nil
}
