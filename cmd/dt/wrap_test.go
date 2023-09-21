package main

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
	"gopkg.in/yaml.v3"
)

func (suite *CmdSuite) TestWrapCommand() {
	t := suite.T()
	require := suite.Require()
	assert := suite.Assert()

	const (
		withLock    = true
		withoutLock = false
	)

	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageName := "test:mytag"

	images, err := tu.AddSampleImagesToRegistry(imageName, u.Host)
	if err != nil {
		t.Fatal(err)
	}

	sb := suite.sb
	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"
	version := "1.0.0"
	scenarioDir, err := filepath.Abs(fmt.Sprintf("../../testdata/scenarios/%s", scenarioName))
	require.NoError(err)

	createSampleChart := func(dest string, withLock bool) string {
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(dest, scenarioName)
		if !withLock {
			// We do not want the lock file to be present, wrap should take care of it
			require.NoError(os.RemoveAll(filepath.Join(chartDir, "Images.lock")))
		}
		return chartDir
	}
	testWrap := func(t *testing.T, inputChart string, outputFile string, expectedLock map[string]interface{},
		generateCarvelBundle bool) {
		// Setup a working directory to look for the wrap when not providing a output-filename
		currentDir, err := os.Getwd()
		require.NoError(err)

		workingDir, err := sb.Mkdir(sb.TempFile(), 0755)
		require.NoError(err)
		defer os.Chdir(currentDir)

		require.NoError(os.Chdir(workingDir))

		var expectedWrapFile string
		args := []string{"wrap", inputChart}
		if outputFile != "" {
			expectedWrapFile = outputFile
			args = append(args, "--output-file", expectedWrapFile)
		} else {
			expectedWrapFile = filepath.Join(workingDir, fmt.Sprintf("%s-%v.wrap.tgz", chartName, version))
		}
		if generateCarvelBundle {
			args = append(args, "--add-carvel-bundle")
		}
		res := dt(args...)
		res.AssertSuccess(t)

		require.FileExists(expectedWrapFile)

		tmpDir := sb.TempFile()
		require.NoError(utils.Untar(expectedWrapFile, tmpDir, utils.TarConfig{StripComponents: 1}))

		imagesDir := filepath.Join(tmpDir, "images")
		require.DirExists(imagesDir)
		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", digestData.Digest.Encoded()))
				assert.FileExists(imgFile)
			}
		}
		lockFile := filepath.Join(tmpDir, "Images.lock")
		assert.FileExists(lockFile)

		if generateCarvelBundle {
			carvelBundleFile := filepath.Join(tmpDir, ".imgpkg/bundle.yml")
			assert.FileExists(carvelBundleFile)
			carvelImagesLockFile := filepath.Join(tmpDir, ".imgpkg/images.yml")
			assert.FileExists(carvelImagesLockFile)
		}

		newData, err := os.ReadFile(lockFile)
		require.NoError(err)
		var newLock map[string]interface{}
		require.NoError(yaml.Unmarshal(newData, &newLock))
		// Clear the timestamp
		newLock["metadata"] = nil

		assert.Equal(expectedLock, newLock)

	}
	testSampleWrap := func(t *testing.T, withLock bool, outputFile string, generateCarvelBundle bool) {
		dest := sb.TempFile()
		chartDir := createSampleChart(dest, withLock)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
		)
		require.NoError(err)
		var expectedLock map[string]interface{}
		require.NoError(yaml.Unmarshal([]byte(data), &expectedLock))

		// Clear the timestamp
		expectedLock["metadata"] = nil

		testWrap(t, chartDir, outputFile, expectedLock, generateCarvelBundle)
	}

	t.Run("Wrap Chart without exiting lock", func(t *testing.T) {
		testSampleWrap(t, withoutLock, "", false)
	})
	t.Run("Wrap Chart with exiting lock", func(t *testing.T) {
		testSampleWrap(t, withLock, "", false)
	})
	t.Run("Wrap Chart From compressed tgz", func(t *testing.T) {
		dest := sb.TempFile()
		chartDir := createSampleChart(dest, withLock)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
		)
		require.NoError(err)
		var expectedLock map[string]interface{}
		require.NoError(yaml.Unmarshal([]byte(data), &expectedLock))

		// Clear the timestamp
		expectedLock["metadata"] = nil

		tarFilename := fmt.Sprintf("%s/chart.tar.gz", sb.TempFile())

		require.NoError(utils.Tar(chartDir, tarFilename, utils.TarConfig{}))
		require.FileExists(tarFilename)

		testWrap(t, tarFilename, "", expectedLock, false)
	})

	t.Run("Wrap Chart From oci", func(t *testing.T) {
		silentLog := log.New(io.Discard, "", 0)

		s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
		defer s.Close()
		u, err := url.Parse(s.URL)
		if err != nil {
			t.Fatal(err)
		}
		ociServerURL := u.Host

		dest := sb.TempFile()
		chartDir := createSampleChart(dest, withLock)

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
		)
		require.NoError(err)
		var expectedLock map[string]interface{}
		require.NoError(yaml.Unmarshal([]byte(data), &expectedLock))

		// Clear the timestamp
		expectedLock["metadata"] = nil

		tarFilename := fmt.Sprintf("%s/chart.tar.gz", sb.TempFile())

		require.NoError(utils.Tar(chartDir, tarFilename, utils.TarConfig{}))
		require.FileExists(tarFilename)
		pushChartURL := fmt.Sprintf("oci://%s/charts", ociServerURL)
		fullChartURL := fmt.Sprintf("%s/%s", pushChartURL, chartName)

		require.NoError(utils.PushChart(tarFilename, pushChartURL))

		testWrap(t, fullChartURL, "", expectedLock, false)
	})

	t.Run("Wrap Chart with custom output filename", func(t *testing.T) {
		tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
		testSampleWrap(t, withLock, tempFilename, false)
		// This should already be handled by testWrap, but make sure it is there
		suite.Assert().FileExists(tempFilename)
	})

	t.Run("Wrap Chart and generate carvel bundle", func(t *testing.T) {
		tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
		testSampleWrap(t, withLock, tempFilename, true) // triggers the Carvel checks
	})
}
