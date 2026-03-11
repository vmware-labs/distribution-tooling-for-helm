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

var sb *tu.Sandbox

func TestMain(m *testing.M) {
	sb = tu.NewSandbox()
	c := m.Run()
	_ = sb.Cleanup()
	os.Exit(c)
}

func TestCreateContainer(t *testing.T) {
	t.Run("Creates container in new directory", func(t *testing.T) {
		destDir := sb.TempFile()
		require.NoFileExists(t, destDir)

		wc, err := CreateContainer(destDir)
		require.NoError(t, err)
		require.NotNil(t, wc)

		assert.Equal(t, destDir, wc.RootDir())
		assert.DirExists(t, destDir)
	})

	t.Run("Creates container when directory already exists", func(t *testing.T) {
		destDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		wc, err := CreateContainer(destDir)
		require.NoError(t, err)
		require.NotNil(t, wc)
		assert.Equal(t, destDir, wc.RootDir())
	})

	t.Run("Fails when parent directory has no write permissions", func(t *testing.T) {
		parentDir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0000))
		require.NoError(t, err)
		defer func() { _ = os.Chmod(parentDir, 0755) }()

		destDir := filepath.Join(parentDir, "container")
		_, err = CreateContainer(destDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create container wrap root directory")
	})
}

func TestLoadContainer(t *testing.T) {
	t.Run("Loads existing directory", func(t *testing.T) {
		dir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		wc, err := LoadContainer(dir)
		require.NoError(t, err)
		require.NotNil(t, wc)
		assert.Equal(t, dir, wc.RootDir())
	})

	t.Run("Returns container even for non-existing directory", func(t *testing.T) {
		nonExisting := sb.TempFile()
		require.NoFileExists(t, nonExisting)

		wc, err := LoadContainer(nonExisting)
		require.NoError(t, err)
		require.NotNil(t, wc)
		assert.Equal(t, nonExisting, wc.RootDir())
	})
}

func TestWrapContainerPaths(t *testing.T) {
	rootDir := "/some/root/dir"
	wc := &wrapContainer{rootDir: rootDir}

	t.Run("LockFilePath returns correct path", func(t *testing.T) {
		expected := filepath.Join(rootDir, imagelock.DefaultImagesLockFileName)
		assert.Equal(t, expected, wc.LockFilePath())
	})

	t.Run("ImageArtifactsDir returns correct path", func(t *testing.T) {
		expected := filepath.Join(rootDir, artifacts.ArtifactsFolder, "images")
		assert.Equal(t, expected, wc.ImageArtifactsDir())
	})

	t.Run("ImagesDir returns correct path", func(t *testing.T) {
		expected := filepath.Join(rootDir, "images")
		assert.Equal(t, expected, wc.ImagesDir())
	})
}

func TestWrapContainerGetImagesLock(t *testing.T) {
	t.Run("Returns ImagesLock from valid lock file", func(t *testing.T) {
		dir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		il := imagelock.NewImagesLock()
		il.Chart.Name = "mycontainer"
		il.Images = imagelock.ImageList{
			{
				Name:  "myapp",
				Image: "registry.io/myorg/myapp:1.0",
				Digests: []imagelock.DigestInfo{
					{Arch: "linux/amd64", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
				},
			},
		}

		lockFile := filepath.Join(dir, imagelock.DefaultImagesLockFileName)
		var buf bytes.Buffer
		require.NoError(t, il.ToYAML(&buf))
		require.NoError(t, os.WriteFile(lockFile, buf.Bytes(), 0644))

		wc := &wrapContainer{rootDir: dir}
		got, err := wc.GetImagesLock()
		require.NoError(t, err)
		require.NotNil(t, got)

		require.Len(t, got.Images, 1)
		assert.Equal(t, "myapp", got.Images[0].Name)
		assert.Equal(t, "registry.io/myorg/myapp:1.0", got.Images[0].Image)
	})

	t.Run("Fails when lock file does not exist", func(t *testing.T) {
		dir, err := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
		require.NoError(t, err)

		wc := &wrapContainer{rootDir: dir}
		_, err = wc.GetImagesLock()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no such file or directory")
	})
}
