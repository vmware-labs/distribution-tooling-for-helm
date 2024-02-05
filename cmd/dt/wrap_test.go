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
	"helm.sh/helm/v3/pkg/repo/repotest"

	"github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/carvel"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/wrapping"
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
	Auth                 tu.Auth
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

func verifyChartWrappedArtifacts(t *testing.T, sb *tu.Sandbox, wrapDir string, images []tu.ImageData, artifactsData map[string][]byte) {
	wrap, err := wrapping.Load(wrapDir)
	require.NoError(t, err)
	artifactsDir := filepath.Join(wrapDir, artifacts.HelmArtifactsFolder)
	require.DirExists(t, artifactsDir)
	require.DirExists(t, filepath.Join(artifactsDir, "images"))
	for _, imgData := range images {
		imageTag := "latest"
		idx := strings.LastIndex(imgData.Image, ":")
		if idx != -1 {
			imageTag = imgData.Image[idx+1:]
		}
		imageArtifactDir := filepath.Join(artifactsDir, fmt.Sprintf("images/%s/%s", wrap.Chart().Name(), imgData.Name))
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
	t.Helper()

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

	if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
		args = append(args, "--username", "username", "--password", "password")
	}

	if len(cfg.Images) == 0 {
		dt(args...).AssertSuccessMatch(t, "No images found in Images.lock")
	} else {
		dt(args...).AssertSuccess(t)
	}
	require.FileExists(t, expectedWrapFile)

	tmpDir := sb.TempFile()
	require.NoError(t, utils.Untar(expectedWrapFile, tmpDir, utils.TarConfig{StripComponents: 1}))

	imagesDir := filepath.Join(tmpDir, "images")
	if len(cfg.Images) == 0 {
		require.NoDirExists(t, imagesDir)
	} else {
		require.DirExists(t, imagesDir)
		for _, imgData := range cfg.Images {
			for _, digestData := range imgData.Digests {
				imgDir := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", digestData.Digest.Encoded()))
				assert.DirExists(t, imgDir)
			}
		}
	}
	wrappedChartDir := filepath.Join(tmpDir, "chart")
	lockFile := filepath.Join(wrappedChartDir, "Images.lock")
	assert.FileExists(t, lockFile)

	if cfg.GenerateCarvelBundle {
		carvelBundleFile := filepath.Join(wrappedChartDir, carvel.CarvelBundleFilePath)
		assert.FileExists(t, carvelBundleFile)
		carvelImagesLockFile := filepath.Join(wrappedChartDir, carvel.CarvelImagesFilePath)
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

	tests := []struct {
		name string
		auth bool
	}{
		{name: "WithoutAuth", auth: false},
		{name: "WithAuth", auth: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var username, password string
			var registryURL string
			if tc.auth {
				srv, err := repotest.NewTempServerWithCleanup(t, "")
				if err != nil {
					t.Fatal(err)
				}
				defer srv.Stop()

				ociSrv, err := repotest.NewOCIServer(t, srv.Root())
				if err != nil {
					t.Fatal(err)
				}
				go ociSrv.ListenAndServe()

				username = "username"
				password = "password"

				registryURL = ociSrv.RegistryURL
			} else {
				silentLog := log.New(io.Discard, "", 0)
				s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
				defer s.Close()

				u, err := url.Parse(s.URL)
				if err != nil {
					t.Fatal(err)
				}
				registryURL = u.Host
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

			images, err := tu.AddSampleImagesToRegistry(imageName, registryURL, tu.WithSignKey(keyFile), tu.WithMetadataDir(metadataDir), tu.WithAuth(username, password))
			if err != nil {
				t.Fatal(err)
			}

			serverURL := registryURL
			scenarioName := "complete-chart"
			chartName := "test"
			version := "1.0.0"
			scenarioDir, err := filepath.Abs(fmt.Sprintf("../../testdata/scenarios/%s", scenarioName))
			require.NoError(err)

			createSampleChart := func(chartDir string, withLock bool) string {
				require.NoError(tu.RenderScenario(scenarioDir, chartDir,
					map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": serverURL},
				))
				if !withLock {
					// We do not want the lock file to be present, wrap should take care of it
					require.NoError(os.RemoveAll(filepath.Join(chartDir, "Images.lock")))
				}
				return chartDir
			}
			testWrap := func(t *testing.T, inputChart string, outputFile string, expectedLock map[string]interface{},
				generateCarvelBundle bool, fetchArtifacts bool, username string, password string) string {
				return testChartWrap(t, sb, inputChart, expectedLock, wrapOpts{
					FetchArtifacts:       fetchArtifacts,
					GenerateCarvelBundle: generateCarvelBundle,
					ChartName:            chartName,
					Version:              version,
					OutputFile:           outputFile,
					ArtifactsMetadata:    metadataArtifacts,
					Images:               images,
					Auth: tu.Auth{
						Username: username,
						Password: password,
					},
				})
			}
			testSampleWrap := func(t *testing.T, withLock bool, outputFile string, generateCarvelBundle bool, fetchArtifacts bool, username string, password string) {
				chartDir := createSampleChart(sb.TempFile(), withLock)

				data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
					map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
				)
				require.NoError(err)
				var expectedLock map[string]interface{}
				require.NoError(yaml.Unmarshal([]byte(data), &expectedLock))

				// Clear the timestamp
				expectedLock["metadata"] = nil

				testWrap(t, chartDir, outputFile, expectedLock, generateCarvelBundle, fetchArtifacts, username, password)
			}

			t.Run("Wrap Chart without existing lock", func(t *testing.T) {
				testSampleWrap(t, withoutLock, "", false, WithoutArtifacts, username, password)
			})
			t.Run("Wrap Chart with existing lock", func(t *testing.T) {
				testSampleWrap(t, withLock, "", false, WithoutArtifacts, username, password)
			})
			t.Run("Wrap Chart From compressed tgz", func(t *testing.T) {
				chartDir := createSampleChart(sb.TempFile(), withLock)

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

				testWrap(t, tarFilename, "", expectedLock, false, WithoutArtifacts, username, password)
			})

			t.Run("Wrap Chart From oci", func(t *testing.T) {
				ociServerURL := registryURL
				if !tc.auth {
					silentLog := log.New(io.Discard, "", 0)
					s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
					defer s.Close()
					u, err := url.Parse(s.URL)
					if err != nil {
						t.Fatal(err)
					}
					ociServerURL = u.Host
				}

				chartDir := createSampleChart(sb.TempFile(), withLock)

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

				require.NoError(artifacts.PushChart(tarFilename, pushChartURL, artifacts.WithRegistryAuth(username, password)))
				t.Run("With artifacts", func(t *testing.T) {
					testWrap(t, fullChartURL, "", expectedLock, false, WithArtifacts, username, password)
				})
				t.Run("Without artifacts", func(t *testing.T) {
					testWrap(t, fullChartURL, "", expectedLock, false, WithoutArtifacts, username, password)
				})
			})

			t.Run("Wrap Chart with custom output filename", func(t *testing.T) {
				tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
				testSampleWrap(t, withLock, tempFilename, false, WithoutArtifacts, username, password)
				// This should already be handled by testWrap, but make sure it is there
				suite.Assert().FileExists(tempFilename)
			})

			t.Run("Wrap Chart and generate carvel bundle", func(t *testing.T) {
				tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())
				testSampleWrap(t, withLock, tempFilename, true, WithoutArtifacts, username, password) // triggers the Carvel checks
			})

			t.Run("Wrap Chart with no images", func(t *testing.T) {
				images = []tu.ImageData{}
				scenarioName = "no-images-chart"
				scenarioDir = fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
				testSampleWrap(t, withLock, "", false, WithoutArtifacts, username, password)
			})
		})
	}
}
