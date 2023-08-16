package imagelock

import (
	"fmt"
	"testing"

	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageList_ToAnnotation(t *testing.T) {
	images := ImageList{
		{
			Image: "app:latest",
			Name:  "app1",
		},
		{
			Image: "blog:v2",
			Name:  "app2",
		},
	}
	t.Run("ImageList serializes as annotation", func(t *testing.T) {
		expected := ""
		for _, img := range images {
			expected += fmt.Sprintf("- name: %s\n  image: %s\n", img.Name, img.Image)
		}
		got, err := images.ToAnnotation()
		require.NoError(t, err)
		assert.Equal(t, tu.MustNormalizeYAML(expected), tu.MustNormalizeYAML(string(got)))
	})
}
