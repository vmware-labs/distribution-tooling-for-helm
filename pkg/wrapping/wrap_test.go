package wrapping

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
)

// newPlainChart renders the plain-chart scenario into a fresh temp directory
// and returns its path.
func newPlainChart(t *testing.T) string {
	t.Helper()
	chartDir := sb.TempFile()
	require.NoError(t, tu.RenderScenario("../../testdata/scenarios/plain-chart", chartDir, map[string]interface{}{}))
	return chartDir
}

func TestCreate(t *testing.T) {
	t.Run("Creates wrap in new directory", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir := sb.TempFile()
		require.NoFileExists(t, destDir)

		w, err := Create(chartDir, destDir)
		require.NoError(t, err)
		require.NotNil(t, w)

		assert.Equal(t, destDir, w.RootDir())
		assert.DirExists(t, destDir)
		assert.DirExists(t, filepath.Join(destDir, "chart"))
	})

	t.Run("Creates wrap when destDir already exists", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		w, err := Create(chartDir, destDir)
		require.NoError(t, err)
		require.NotNil(t, w)
		assert.Equal(t, destDir, w.RootDir())
	})

	t.Run("Fails when chartSrc is not a valid Helm chart", func(t *testing.T) {
		notAChart, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)
		destDir := sb.TempFile()

		_, err = Create(notAChart, destDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load Helm chart")
	})

	t.Run("Fails when chart sub-directory already exists in destDir", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)
		// Pre-create the chart/ sub-dir to trigger the conflict
		_, err = sb.Mkdir(filepath.Join(destDir, "chart"), os.FileMode(0755))
		require.NoError(t, err)

		_, err = Create(chartDir, destDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Fails when parent directory has no write permissions", func(t *testing.T) {
		chartDir := newPlainChart(t)
		parentDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0000))
		require.NoError(t, err)
		defer func() { _ = os.Chmod(parentDir, 0755) }()

		destDir := filepath.Join(parentDir, "wrap")
		_, err = Create(chartDir, destDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create wrap root directory")
	})
}

func TestLoad(t *testing.T) {
	t.Run("Loads wrap from directory with valid chart sub-dir", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir := sb.TempFile()

		// First create a proper wrap on disk, then load it
		_, err := Create(chartDir, destDir)
		require.NoError(t, err)

		w, err := Load(destDir)
		require.NoError(t, err)
		require.NotNil(t, w)
		assert.Equal(t, destDir, w.RootDir())
	})

	t.Run("Fails when chart sub-directory does not exist", func(t *testing.T) {
		emptyDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		_, err = Load(emptyDir)
		require.Error(t, err)
	})
}

func TestWrapPaths(t *testing.T) {
	chartDir := newPlainChart(t)
	destDir := sb.TempFile()
	_, err := Create(chartDir, destDir)
	require.NoError(t, err)

	loaded, err := Load(destDir)
	require.NoError(t, err)
	w := loaded.(*wrap)

	t.Run("RootDir returns correct path", func(t *testing.T) {
		assert.Equal(t, destDir, w.RootDir())
	})

	t.Run("ChartDir returns correct path", func(t *testing.T) {
		assert.Equal(t, filepath.Join(destDir, "chart"), w.ChartDir())
	})

	t.Run("LockFilePath returns correct path", func(t *testing.T) {
		expected := filepath.Join(destDir, "chart", imagelock.DefaultImagesLockFileName)
		assert.Equal(t, expected, w.LockFilePath())
	})

	t.Run("ImageArtifactsDir returns correct path", func(t *testing.T) {
		expected := filepath.Join(destDir, artifacts.ArtifactsFolder, "images")
		assert.Equal(t, expected, w.ImageArtifactsDir())
	})

	t.Run("ImagesDir returns correct path", func(t *testing.T) {
		expected := filepath.Join(destDir, "images")
		assert.Equal(t, expected, w.ImagesDir())
	})

	t.Run("AbsFilePath returns correct path", func(t *testing.T) {
		assert.Equal(t, filepath.Join(destDir, "foo"), w.AbsFilePath("foo"))
	})
}

func TestWrapGetImagesLock(t *testing.T) {
	t.Run("Returns ImagesLock from valid lock file", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir := sb.TempFile()
		_, err := Create(chartDir, destDir)
		require.NoError(t, err)

		il := imagelock.NewImagesLock()
		il.Chart.Name = "wordpress"
		il.Images = imagelock.ImageList{
			{
				Name:  "wordpress",
				Image: "registry.io/bitnami/wordpress:6.0",
				Digests: []imagelock.DigestInfo{
					{Arch: "linux/amd64", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
				},
			},
		}

		lockFile := filepath.Join(destDir, "chart", imagelock.DefaultImagesLockFileName)
		var buf bytes.Buffer
		require.NoError(t, il.ToYAML(&buf))
		require.NoError(t, os.WriteFile(lockFile, buf.Bytes(), 0644))

		w, err := Load(destDir)
		require.NoError(t, err)

		got, err := w.GetImagesLock()
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Len(t, got.Images, 1)
		assert.Equal(t, "wordpress", got.Images[0].Name)
		assert.Equal(t, "registry.io/bitnami/wordpress:6.0", got.Images[0].Image)
	})

	t.Run("Fails when lock file does not exist", func(t *testing.T) {
		chartDir := newPlainChart(t)
		destDir := sb.TempFile()
		_, err := Create(chartDir, destDir)
		require.NoError(t, err)

		w, err := Load(destDir)
		require.NoError(t, err)

		_, err = w.GetImagesLock()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})
}
