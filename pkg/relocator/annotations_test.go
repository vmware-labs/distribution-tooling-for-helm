package relocator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func TestRelocateAnnotations(t *testing.T) {
	chartDir := sb.TempFile()
	serverURL := "localhost"

	require.NoError(t, tu.RenderScenario("../../testdata/scenarios/chart1", chartDir, map[string]interface{}{"ServerURL": serverURL}))

	newServerURL := "test.example.com"
	expectedAnnotations, err := tu.RenderTemplateFile("../../testdata/scenarios/chart1/images.partial.tmpl", map[string]string{"ServerURL": newServerURL})
	require.NoError(t, err)

	expectedAnnotations = strings.TrimSpace(expectedAnnotations)

	newAnnotations, err := RelocateAnnotations(chartDir, newServerURL)
	require.NoError(t, err)

	assert.Equal(t, strings.TrimSpace(expectedAnnotations), strings.TrimSpace(newAnnotations))
}
