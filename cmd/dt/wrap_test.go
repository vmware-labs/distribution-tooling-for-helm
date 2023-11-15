package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vmware-labs/distribution-tooling-for-helm/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/carvel"
	"github.com/vmware-labs/distribution-tooling-for-helm/chartutils"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
	"gopkg.in/yaml.v3"
)

const (
	WithArtifacts    = true
	WithoutArtifacts = false
)

type wrapOpts struct {
	FetchArtifacts       bool
	GenerateCarvelBundle bool
	ChartName            string
	Version              string
	OutputFile           string
	SkipExpectedLock     bool
	Images               []tu.ImageData
	ArtifactsMetadata    map[string][]byte
}

func verifyArtifactsContents(t *testing.T, sb *tu.Sandbox, dir string, artifactsData map[string][]byte) {
	plainMetadataDir, err := sb.Mkdir(sb.TempFile(), 0755)
	require.NoError(t, err)
	require.NoError(t, testutil.UnpackOCILayout(context.Background(), dir, plainMetadataDir))

	for fileName, data := range artifactsData {
		got, err := os.ReadFile(filepath.Join(plainMetadataDir, fileName))
		require.NoError(t, err)
		require.Equal(t, data, got)
	}
}

func verifyChartWrappedArtifacts(t *testing.T, sb *tu.Sandbox, chartDir string, images []tu.ImageData, artifactsData map[string][]byte) {
	chart, err := chartutils.LoadChart(chartDir)
	require.NoError(t, err)
	artifactsDir := filepath.Join(chartDir, artifacts.HelmArtifactsFolder)
	require.DirExists(t, artifactsDir)
	require.DirExists(t, filepath.Join(artifactsDir, "images"))
	for _, imgData := range images {
		imageTag := "latest"
		idx := strings.LastIndex(imgData.Image, ":")
		if idx != -1 {
			imageTag = imgData.Image[idx+1:]
		}
		imageArtifactDir := filepath.Join(artifactsDir, fmt.Sprintf("images/%s/%s", chart.Metadata.Name, imgData.Name))
		require.DirExists(t, imageArtifactDir)
		for _, dir := range []string{"sig", "metadata", "metadata.sig"} {
			imageArtifactDir := filepath.Join(imageArtifactDir, fmt.Sprintf("%s.%s", imageTag, dir))
			require.DirExists(t, imageArtifactDir)
			// Basic validation of the oci-layout dir
			for _, f := range []string{"index.json", "oci-layout"} {
				require.FileExists(t, filepath.Join(imageArtifactDir, f))
			}
			require.DirExists(t, filepath.Join(imageArtifactDir, "blobs"))

			// For the "metadata" dir, check the bundle assets match what we provided
			if dir == "metadata" {
				verifyArtifactsContents(t, sb, imageArtifactDir, artifactsData)
			}
		}
	}
}

func testChartWrap(t *testing.T, sb *tu.Sandbox, inputChart string, expectedLock map[string]interface{},
	cfg wrapOpts) string {

	// Setup a working directory to look for the wrap when not providing a output-filename
	currentDir, err := os.Getwd()
	require.NoError(t, err)

	workingDir, err := sb.Mkdir(sb.TempFile(), 0755)
	require.NoError(t, err)
	defer os.Chdir(currentDir)

	require.NoError(t, os.Chdir(workingDir))

	var expectedWrapFile string
	args := []string{"wrap", inputChart, "--use-plain-http"}
	if cfg.OutputFile != "" {
		expectedWrapFile = cfg.OutputFile
		args = append(args, "--output-file", expectedWrapFile)
	} else {
		expectedWrapFile = filepath.Join(workingDir, fmt.Sprintf("%s-%v.wrap.tgz", cfg.ChartName, cfg.Version))
	}
	if cfg.GenerateCarvelBundle {
		args = append(args, "--add-carvel-bundle")
	}
	if cfg.FetchArtifacts {
		args = append(args, "--fetch-artifacts")
	}
	dt(args...).AssertSuccess(t)

	require.FileExists(t, expectedWrapFile)

	tmpDir := sb.TempFile()
	require.NoError(t, utils.Untar(expectedWrapFile, tmpDir, utils.TarConfig{StripComponents: 1}))

	imagesDir := filepath.Join(tmpDir, "images")
	require.DirExists(t, imagesDir)
	for _, imgData := range cfg.Images {
		for _, digestData := range imgData.Digests {
			imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", digestData.Digest.Encoded()))
			assert.FileExists(t, imgFile)
		}
	}
	lockFile := filepath.Join(tmpDir, "Images.lock")
	assert.FileExists(t, lockFile)

	if cfg.GenerateCarvelBundle {
		carvelBundleFile := filepath.Join(tmpDir, carvel.CarvelBundleFilePath)
		assert.FileExists(t, carvelBundleFile)
		carvelImagesLockFile := filepath.Join(tmpDir, carvel.CarvelImagesFilePath)
		assert.FileExists(t, carvelImagesLockFile)
	}

	newData, err := os.ReadFile(lockFile)
	require.NoError(t, err)
	var newLock map[string]interface{}
	require.NoError(t, yaml.Unmarshal(newData, &newLock))
	// Clear the timestamp
	newLock["metadata"] = nil
	if !cfg.SkipExpectedLock {
		assert.Equal(t, expectedLock, newLock)
	}

	if cfg.FetchArtifacts {
		if len(cfg.ArtifactsMetadata) > 0 {
			verifyChartWrappedArtifacts(t, sb, tmpDir, cfg.Images, cfg.ArtifactsMetadata)
		}
	} else {
		// We did not requested fetching artifacts. Make sure they are not grabbed
		assert.NoDirExists(t, filepath.Join(tmpDir, artifacts.HelmArtifactsFolder))
	}
	return tmpDir
}

func (suite *CmdSuite) TestWrapCommand() {
	t := suite.T()
	require := suite.Require()

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
	imageTag := "mytag"
	imageName := fmt.Sprintf("test:%s", imageTag)

	sb := suite.sb

	certDir, err := sb.Mkdir(sb.TempFile(), 0755)
	require.NoError(err)

	keyFile, _, err := tu.GenerateCosignCertificateFiles(certDir)
	require.NoError(err)

	metadataDir, err := sb.Mkdir(sb.TempFile(), 0755)
	require.NoError(err)

	metdataFileText := "this is a sample text"

	metadataArtifacts := map[string][]byte{
		"metadata.txt": []byte(metdataFileText),
	}
	for fileName, data := range metadataArtifacts {
		_, err := sb.Write(filepath.Join(metadataDir, fileName), string(data))
		require.NoError(err)
	}

	images, err := tu.AddSampleImagesToRegistry(imageName, u.Host, tu.WithSignKey(keyFile), tu.WithMetadataDir(metadataDir))
	if err != nil {
		t.Fatal(err)
	}

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
		generateCarvelBundle bool, fetchArtifacts bool) string {
		return testChartWrap(t, sb, inputChart, expectedLock, wrapOpts{
			FetchArtifacts:       fetchArtifacts,
			GenerateCarvelBundle: generateCarvelBundle,
			ChartName:            chartName,
			Version:              version,
			OutputFile:           outputFile,
			ArtifactsMetadata:    metadataArtifacts,
			Images:               images,
		})
	}
	testSampleWrap := func(t *testing.T, withLock bool, outputFile string, generateCarvelBundle bool, fetchArtifacts bool) {
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

		testWrap(t, chartDir, outputFile, expectedLock, generateCarvelBundle, fetchArtifacts)
	}

	t.Run("Wrap Chart without exiting lock", func(t *testing.T) {
		testSampleWrap(t, withoutLock, "", false, false)
	})
	t.Run("Wrap Chart with exiting lock", func(t *testing.T) {
		testSampleWrap(t, withLock, "", false, false)
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

		testWrap(t, tarFilename, "", expectedLock, false, false)
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

		require.NoError(artifacts.PushChart(tarFilename, pushChartURL))
		t.Run("With artifacts", func(t *testing.T) {
			testWrap(t, fullChartURL, "", expectedLock, false, WithArtifacts)
		})
		t.Run("Withoout artifacts", func(t *testing.T) {
			testWrap(t, fullChartURL, "", expectedLock, false, WithoutArtifacts)
		})
	})

	t.Run("Wrap Chart with custom output filename", func(t *testing.T) {
		tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
		testSampleWrap(t, withLock, tempFilename, false, false)
		// This should already be handled by testWrap, but make sure it is there
		suite.Assert().FileExists(tempFilename)
	})

	t.Run("Wrap Chart and generate carvel bundle", func(t *testing.T) {
		tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
		testSampleWrap(t, withLock, tempFilename, true, false) // triggers the Carvel checks
	})
}
