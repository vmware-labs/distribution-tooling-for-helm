package main

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func (suite *CmdSuite) TestPushCommand() {
	t := suite.T()
	sb := suite.sb
	require := suite.Require()
	assert := suite.Assert()

	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	serverURL := u.Host

	t.Run("Handle errors", func(t *testing.T) {
		t.Run("Handle missing Images.lock", func(t *testing.T) {
			chartName := "test"
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			chartDir := filepath.Join(dest, scenarioName)
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to open Images.lock file:.*no such file or directory`))
		})
		t.Run("Handle malformed Helm chart", func(t *testing.T) {
			dt("images", "push", sb.TempFile()).AssertErrorMatch(t, regexp.MustCompile(`cannot determine Helm chart root`))
		})
		t.Run("Handle malformed Images.lock", func(t *testing.T) {
			chartName := "test"
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			chartDir := filepath.Join(dest, scenarioName)
			require.NoError(os.WriteFile(filepath.Join(chartDir, imagelock.DefaultImagesLockFileName), []byte("malformed lock"), 0644))
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to load Images.lock`))
		})
		t.Run("Handle failing to push images", func(t *testing.T) {
			chartName := "test"
			scenarioName := "chart1"
			serverURL := "example.com"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			dest := sb.TempFile()
			require.NoError(tu.RenderScenario(scenarioDir, dest,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			chartDir := filepath.Join(dest, scenarioName)
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`(?i)failed to push images`))
		})
	})
	t.Run("Pushing works", func(t *testing.T) {
		scenarioName := "complete-chart"
		chartName := "test"

		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
		imageName := "test:mytag"

		imageData := tu.ImageData{Name: "test", Image: "test:mytag"}
		architectures := []string{
			"linux/amd64",
			"linux/arm",
		}
		craneImgs, err := tu.CreateSampleImages(&imageData, architectures)

		if err != nil {
			t.Fatal(err)
		}

		require.Equal(len(architectures), len(imageData.Digests))

		images := []tu.ImageData{imageData}
		dest := sb.TempFile()
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{
				"ServerURL": serverURL, "Images": images,
				"Name": chartName, "RepositoryURL": serverURL,
			},
		))

		chartDir := filepath.Join(dest, scenarioName)
		imagesDir := filepath.Join(chartDir, "images")
		require.NoError(os.MkdirAll(imagesDir, 0755))
		for _, img := range craneImgs {
			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", d.Hex))
			if err := crane.Save(img, imageName, imgFile); err != nil {
				t.Fatal(err)
			}
		}

		t.Run("Push images", func(t *testing.T) {
			require.NoError(err)
			dt("images", "push", chartDir).AssertSuccessMatch(t, "")

			// Verify the images were pushed
			for _, img := range images {
				src := fmt.Sprintf("%s/%s", u.Host, img.Image)
				remoteDigests, err := tu.ReadRemoteImageManifest(src)
				if err != nil {
					t.Fatal(err)
				}
				for _, dgstData := range img.Digests {
					assert.Equal(dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
				}
			}
		})
	})

}
