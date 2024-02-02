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
	"regexp"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
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
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to open Images.lock file:.*no such file or directory`))
		})
		t.Run("Handle malformed Helm chart", func(t *testing.T) {
			dt("images", "push", sb.TempFile()).AssertErrorMatch(t, regexp.MustCompile(`failed to load Helm chart`))
		})
		t.Run("Handle malformed Images.lock", func(t *testing.T) {
			chartName := "test"
			scenarioName := "plain-chart"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			require.NoError(os.WriteFile(filepath.Join(chartDir, imagelock.DefaultImagesLockFileName), []byte("malformed lock"), 0644))
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`failed to load Images.lock`))
		})
		t.Run("Handle failing to push images", func(t *testing.T) {
			chartName := "test"
			scenarioName := "chart1"
			serverURL := "example.com"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			chartDir := sb.TempFile()

			require.NoError(tu.RenderScenario(scenarioDir, chartDir,
				map[string]interface{}{
					"ServerURL": serverURL, "Images": nil,
					"Name": chartName, "RepositoryURL": serverURL,
				},
			))
			dt("images", "push", chartDir).AssertErrorMatch(t, regexp.MustCompile(`(?i)failed to push images`))
		})
	})
	t.Run("Pushing works", func(t *testing.T) {
		scenarioName := "complete-chart"
		chartName := "test"

		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

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
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{
				"ServerURL": serverURL, "Images": images,
				"Name": chartName, "RepositoryURL": serverURL,
			},
		))

		imagesDir := filepath.Join(chartDir, "images")
		require.NoError(os.MkdirAll(imagesDir, 0755))
		for _, img := range craneImgs {
			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			imgDir := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", d.Hex))
			if err := crane.SaveOCI(img, imgDir); err != nil {
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

	t.Run("Pushing works with login", func(t *testing.T) {
		ctx := context.Background()
		containerReq := testcontainers.ContainerRequest{
			Image:        "registry:2",
			ExposedPorts: []string{"5000/tcp"},
			WaitingFor:   wait.ForListeningPort("5000/tcp"),
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      "../../testdata/auth/htpasswd",
					ContainerFilePath: "/htpasswd",
					FileMode:          0o600,
				},
			},
			Env: map[string]string{
				"REGISTRY_AUTH":                "htpasswd",
				"REGISTRY_AUTH_HTPASSWD_REALM": "Registry Realm",
				"REGISTRY_AUTH_HTPASSWD_PATH":  "/htpasswd",
			},
		}
		registry, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: containerReq,
			Started:          true,
		})
		if err != nil {
			t.Fatalf("failed to create container: %v", err)
		}

		mappedPort, err := registry.MappedPort(ctx, "5000/tcp")
		if err != nil {
			t.Fatalf("failed to get port: %v", err)
		}
		registryURI := fmt.Sprintf("127.0.0.1:%s", mappedPort.Port())

		dt("auth", "login", registryURI, "-u", "testuser", "-p", "testpassword").AssertSuccessMatch(t, "logged in via")

		t.Cleanup(func() {
			dt("auth", "logout", registryURI).AssertSuccessMatch(t, "logged out via")
			if err := registry.Terminate(ctx); err != nil {
				t.Fatalf("failed to terminate container: %v", err)
			}
		})
		scenarioName := "complete-chart"
		chartName := "test"

		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

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
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{
				"ServerURL": registryURI, "Images": images,
				"Name": chartName, "RepositoryURL": registryURI,
			},
		))

		imagesDir := filepath.Join(chartDir, "images")
		require.NoError(os.MkdirAll(imagesDir, 0755))
		for _, img := range craneImgs {
			d, err := img.Digest()
			if err != nil {
				t.Fatal(err)
			}
			imgDir := filepath.Join(imagesDir, fmt.Sprintf("%s.layout", d.Hex))
			if err := crane.SaveOCI(img, imgDir); err != nil {
				t.Fatal(err)
			}
		}

		t.Run("Push images", func(t *testing.T) {
			require.NoError(err)
			dt("images", "push", chartDir).AssertSuccessMatch(t, "")

			// Verify the images were pushed
			for _, img := range images {
				src := fmt.Sprintf("%s/%s", registryURI, img.Image)
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
