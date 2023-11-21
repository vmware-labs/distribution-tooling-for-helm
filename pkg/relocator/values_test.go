package relocator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func TestRelocateValues(t *testing.T) {

	chartDir := sb.TempFile()
	serverURL := "localhost"

	require.NoError(t, tu.RenderScenario("../../testdata/scenarios/chart1", chartDir, map[string]interface{}{"ServerURL": serverURL}))

	newServerURL := "test.example.com"
	data, err := tu.RenderTemplateFile("../../testdata/scenarios/chart1/values.yaml.tmpl", map[string]string{"ServerURL": newServerURL})
	require.NoError(t, err)

	expectedValues, err := tu.NormalizeYAML(data)
	require.NoError(t, err)

	newValues, err := RelocateValues(chartDir, newServerURL)
	require.NoError(t, err)
	newValues, err = tu.NormalizeYAML(newValues)
	require.NoError(t, err)

	assert.Equal(t, expectedValues, newValues)
}
