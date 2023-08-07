package relocator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func TestRelocateValues(t *testing.T) {

	dest := sb.TempFile()
	serverURL := "localhost"
	require.NoError(t, tu.RenderScenario("../testdata/scenarios/chart1", dest, map[string]interface{}{"ServerURL": serverURL}))
	chartDir := filepath.Join(dest, "chart1")

	newServerURL := "test.example.com"
	data, err := tu.RenderTemplateFile("../testdata/scenarios/chart1/values.yaml.tmpl", map[string]string{"ServerURL": newServerURL})
	require.NoError(t, err)

	expectedValues, err := normalizeYAML(data)
	require.NoError(t, err)

	newValues, err := RelocateValues(chartDir, newServerURL)
	require.NoError(t, err)
	newValues, err = normalizeYAML(newValues)
	require.NoError(t, err)

	assert.Equal(t, expectedValues, newValues)
}
