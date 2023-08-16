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

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/utils"
)

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

		targetRegistry := fmt.Sprintf("%s/new-images", serverURL)
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
			utils.RemoteChartExist(fmt.Sprintf("oci://%s/%s", targetRegistry, chartName), version),
			"chart should exist in the repository",
		)
	})
}
