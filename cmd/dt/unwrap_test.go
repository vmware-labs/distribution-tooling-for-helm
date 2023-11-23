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

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-labs/distribution-tooling-for-helm/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

type unwrapOpts struct {
	FetchedArtifacts  bool
	PublicKey         string
	Images            []tu.ImageData
	ChartName         string
	Version           string
	ArtifactsMetadata map[string][]byte
}

func testChartUnwrap(t *testing.T, sb *tu.Sandbox, inputChart string, targetRegistry string, srcRegistry string, cfg unwrapOpts) {
	dt("unwrap", "--log-level", "debug", "--plain", "--yes", inputChart, targetRegistry).AssertSuccessMatch(t, "")
	assert.True(t,
		artifacts.RemoteChartExist(fmt.Sprintf("oci://%s/%s", targetRegistry, cfg.ChartName), cfg.Version),
		"chart should exist in the repository",
	)

	normalizedSrcRegistry := srcRegistry
	if !strings.Contains(normalizedSrcRegistry, "://") {
		normalizedSrcRegistry = "http://" + normalizedSrcRegistry
	}
	u, err := url.Parse(normalizedSrcRegistry)
	require.NoError(t, err)

	path := u.Path

	relocatedRegistryPath := targetRegistry
	if path != "" && path != "/" {
		relocatedRegistryPath = fmt.Sprintf("%s/%s", relocatedRegistryPath, strings.Trim(filepath.Base(path), "/"))

	}

	// Verify the images were pushed
	for _, img := range cfg.Images {
		src := fmt.Sprintf("%s/%s", relocatedRegistryPath, img.Image)
		remoteDigests, err := tu.ReadRemoteImageManifest(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, dgstData := range img.Digests {
			assert.Equal(t, dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
		}

		tagsInfo := map[string]string{"main": "", "metadata": ""}
		tags, err := artifacts.ListTags(context.Background(), fmt.Sprintf("%s/%s", srcRegistry, "test"))
		require.NoError(t, err)

		for _, tag := range tags {
			if tag == "latest" {
				tagsInfo["main"] = tag
			} else if strings.HasSuffix(tag, ".metadata") {
				tagsInfo["metadata"] = tag
			}
		}
		for _, k := range []string{"main", "metadata"} {
			v := tagsInfo[k]
			if v == "" {
				assert.Fail(t, "Tag %q should not be empty", k)
				continue
			}
			if cfg.PublicKey != "" {
				assert.NoError(t, testutil.CosignVerifyImage(fmt.Sprintf("%s:%s", src, v), cfg.PublicKey), "Signature for %q failed", src)
			}
		}
		// Verify the metadata
		if cfg.FetchedArtifacts {
			ociMetadataDir, err := sb.Mkdir(sb.TempFile(), 0755)
			require.NoError(t, err)
			require.NoError(t, testutil.PullArtifact(context.Background(), fmt.Sprintf("%s:%s", src, tagsInfo["metadata"]), ociMetadataDir))

			verifyArtifactsContents(t, sb, ociMetadataDir, cfg.ArtifactsMetadata)
		}
	}
}

func writeSampleImages(imageName string, imageTag string, dir string) ([]tu.ImageData, error) {
	_ = os.MkdirAll(dir, 0755)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)
	imageData := tu.ImageData{Name: imageName, Image: fullImageName}

	craneImages, err := tu.CreateSampleImages(&imageData, []string{
		"linux/amd64",
		"linux/arm64",
	})

	if err != nil {
		return nil, err
	}

	for _, img := range craneImages {
		d, err := img.Digest()
		if err != nil {
			return nil, fmt.Errorf("failed to get image digest: %w", err)
		}

		imgFileName := filepath.Join(dir, fmt.Sprintf("%s.tar", d.Hex))

		if err := crane.Save(img, fullImageName, imgFileName); err != nil {
			return nil, fmt.Errorf("failed to save image %q to %q: %w", fullImageName, imgFileName, err)
		}

	}
	return []tu.ImageData{imageData}, nil
}

func (suite *CmdSuite) TestUnwrapCommand() {
	t := suite.T()
	silentLog := log.New(io.Discard, "", 0)

	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageName := "test"
	imageTag := "mytag"

	sb := suite.sb
	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"
	version := "1.0.0"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	currentRegIdx := 0

	newTargetRegistry := func(name string) string {
		return fmt.Sprintf("%s/%s", serverURL, name)
	}
	newUniqueTargetRegistry := func() string {
		currentRegIdx++
		return newTargetRegistry(fmt.Sprintf("new-images-%d", currentRegIdx))
	}
	t.Run("Unwrap Chart", func(t *testing.T) {
		require := suite.Require()
		assert := suite.Assert()
		dest := sb.TempFile()
		chartDir := filepath.Join(dest, scenarioName)

		images, err := writeSampleImages(imageName, imageTag, filepath.Join(chartDir, "images"))
		require.NoError(err)

		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": serverURL},
		))

		data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
		)
		require.NoError(err)
		require.NoError(os.WriteFile(filepath.Join(chartDir, "Images.lock"), []byte(data), 0755))

		targetRegistry := newUniqueTargetRegistry()
		dt("unwrap", "--plain", "--yes", chartDir, targetRegistry).AssertSuccessMatch(suite.T(), "")

		// Verify the images were pushed
		for _, img := range images {
			src := fmt.Sprintf("%s/%s", targetRegistry, img.Image)
			remoteDigests, err := tu.ReadRemoteImageManifest(src)
			if err != nil {
				t.Fatal(err)
			}
			for _, dgstData := range img.Digests {
				assert.Equal(dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
			}
		}
		assert.True(
			artifacts.RemoteChartExist(fmt.Sprintf("oci://%s/%s", targetRegistry, chartName), version),
			"chart should exist in the repository",
		)
	})
}
func (suite *CmdSuite) TestEndToEnd() {
	t := suite.T()
	silentLog := log.New(io.Discard, "", 0)

	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageName := "test"

	sb := suite.sb
	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"
	version := "1.0.0"
	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
	currentRegIdx := 0

	newTargetRegistry := func(name string) string {
		return fmt.Sprintf("%s/%s", serverURL, name)
	}
	newUniqueTargetRegistry := func() string {
		currentRegIdx++
		return newTargetRegistry(fmt.Sprintf("new-images-%d", currentRegIdx))
	}
	t.Run("Wrap and unwrap Chart", func(t *testing.T) {
		require := suite.Require()
		dest := sb.TempFile()
		chartDir := filepath.Join(dest, scenarioName)

		srcRegistryNamespace := "wrap-unwrap-test"
		srcRegistry := newTargetRegistry(srcRegistryNamespace)
		targetRegistry := newUniqueTargetRegistry()

		certDir, err := sb.Mkdir(sb.TempFile(), 0755)
		require.NoError(err)

		keyFile, pubKey, err := tu.GenerateCosignCertificateFiles(certDir)
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

		images, err := tu.AddSampleImagesToRegistry(imageName, srcRegistry, tu.WithSignKey(keyFile), tu.WithMetadataDir(metadataDir))
		if err != nil {
			require.NoError(err)
		}

		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": srcRegistry, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": srcRegistry},
		))

		tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())

		testChartWrap(t, sb, chartDir, nil, wrapOpts{
			FetchArtifacts:       true,
			GenerateCarvelBundle: false,
			ChartName:            chartName,
			Version:              version,
			OutputFile:           tempFilename,
			SkipExpectedLock:     true,
			Images:               images,
			ArtifactsMetadata:    metadataArtifacts,
		})

		testChartUnwrap(t, sb, tempFilename, targetRegistry, srcRegistry, unwrapOpts{
			FetchedArtifacts: true, Images: images, PublicKey: pubKey,
			ArtifactsMetadata: metadataArtifacts,
			ChartName:         chartName,
			Version:           version,
		})

	})

}
