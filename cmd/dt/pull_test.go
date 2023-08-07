package main

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

func (suite *CmdSuite) TestPullCommand() {
	t := suite.T()
	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	imageName := "test:mytag"
	src := fmt.Sprintf("%s/%s", u.Host, imageName)
	imageData := tu.ImageData{Name: "test", Image: "test:mytag"}

	imgs := []mutate.IndexAddendum{}

	for _, plat := range []string{
		"linux/amd64",
		"linux/arm",
	} {
		img, err := crane.Image(map[string][]byte{
			"platform.txt": []byte(fmt.Sprintf("Image: %s ; plaform: %s", imageName, plat)),
		})
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(plat, "/")
		imgs = append(imgs, mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           parts[0],
					Architecture: parts[1],
				},
			},
		})
		d, err := img.Digest()
		if err != nil {
			t.Fatal(err)
		}
		imageData.Digests = append(imageData.Digests, tu.DigestData{Arch: plat, Digest: digest.Digest(d.String())})
	}

	idx := mutate.AppendManifests(empty.Index, imgs...)

	ref, err := name.ParseReference(src)
	if err != nil {
		t.Fatal(err)
	}

	if err := remote.WriteIndex(ref, idx); err != nil {
		t.Fatal(err)
	}

	images := []tu.ImageData{imageData}
	sb := suite.sb
	require := suite.Require()
	serverURL := u.Host
	scenarioName := "complete-chart"
	chartName := "test"

	scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

	suite.T().Run("Pulls images", func(t *testing.T) {
		dest := sb.TempFile()
		require.NoError(tu.RenderScenario(scenarioDir, dest,
			map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "RepositoryURL": serverURL},
		))
		chartDir := filepath.Join(dest, scenarioName)

		suite.Require().NoError(err)
		dt("images", "pull", chartDir).AssertSuccessMatch(suite.T(), "")
		imagesDir := filepath.Join(chartDir, "images")
		suite.Require().DirExists(imagesDir)
		for _, imgData := range images {
			for _, digestData := range imgData.Digests {
				imgFile := filepath.Join(imagesDir, fmt.Sprintf("%s.tar", digestData.Digest.Encoded()))
				suite.Assert().FileExists(imgFile)
			}
		}
	})
}
