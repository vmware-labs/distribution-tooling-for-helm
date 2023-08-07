package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	"github.com/vmware-labs/distribution-tooling-for-helm/imagelock"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func readRemoteManifest(src string) (map[string]tu.DigestData, error) {
	o := crane.GetOptions()

	ref, err := name.ParseReference(src, o.Name...)

	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %q: %w", src, err)
	}
	desc, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote image: %w", err)
	}

	var idx v1.IndexManifest
	if err := json.Unmarshal(desc.Manifest, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse images data")
	}
	digests := make(map[string]tu.DigestData, 0)

	var allErrors error
	for _, img := range idx.Manifests {
		// Skip attestations
		if img.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
			continue
		}
		switch img.MediaType {
		case types.OCIManifestSchema1, types.DockerManifestSchema2:
			if img.Platform == nil {
				continue
			}

			arch := fmt.Sprintf("%s/%s", img.Platform.OS, img.Platform.Architecture)
			imgDigest := tu.DigestData{
				Digest: digest.Digest(img.Digest.String()),
				Arch:   arch,
			}
			digests[arch] = imgDigest
		default:
			allErrors = errors.Join(allErrors, fmt.Errorf("unknown media type %q", img.MediaType))
			continue
		}
	}
	return digests, allErrors
}

func createImages(imageData *tu.ImageData, archs []string) ([]v1.Image, error) {
	craneImgs := []v1.Image{}
	imageName := imageData.Image

	for _, plat := range archs {

		img, err := crane.Image(map[string][]byte{
			"platform.txt": []byte(fmt.Sprintf("Image: %s ; plaform: %s", imageName, plat)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create image: %w", err)
		}
		parts := strings.Split(plat, "/")
		img, err = mutate.ConfigFile(img, &v1.ConfigFile{Architecture: parts[1], OS: parts[0]})
		if err != nil {
			return nil, fmt.Errorf("cannot mutatle image config file: %w", err)
		}

		img, err = mutate.Canonical(img)
		if err != nil {
			return nil, fmt.Errorf("failed to canonicalize image: %w", err)
		}

		d, err := img.Digest()
		if err != nil {
			return nil, fmt.Errorf("failed to get image digest: %w", err)
		}
		craneImgs = append(craneImgs, img)

		imageData.Digests = append(imageData.Digests, tu.DigestData{Arch: plat, Digest: digest.Digest(d.String())})
	}
	return craneImgs, nil
}

func (suite *CmdSuite) TestPushCommand() {
	t := suite.T()
	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	serverURL := u.Host

	sb := suite.sb
	require := suite.Require()
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
		craneImgs, err := createImages(&imageData, architectures)

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

		suite.T().Run("Push images", func(t *testing.T) {
			suite.Require().NoError(err)
			dt("images", "push", chartDir).AssertSuccessMatch(t, "")

			// Verify the images were pushed
			for _, img := range images {
				src := fmt.Sprintf("%s/%s", u.Host, img.Image)
				remoteDigests, err := readRemoteManifest(src)
				if err != nil {
					t.Fatal(err)
				}
				for _, dgstData := range img.Digests {
					suite.Assert().Equal(dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
				}
			}
			//	at("images", "verify", chartDir).AssertSuccessMatch(suite.T(), "")

		})
	})

}
