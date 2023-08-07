// Package testutil implements functions used to test the different packages
package testutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/opencontainers/go-digest"
)

// TestServer defines a images registry for testing
type TestServer struct {
	ServerURL    string
	s            *httptest.Server
	responsesMap map[string]response
}

// DigestData defines Digest information for an Architecture
type DigestData struct {
	Arch   string
	Digest digest.Digest
}

// ImageData defines information for a docker image
type ImageData struct {
	Name    string
	Image   string
	Digests []DigestData
}

// AddImage adds information for an image to the server so it can be later queried
func (s *TestServer) AddImage(img *ImageData) error {
	imgID := img.Image
	parts := strings.SplitN(imgID, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("failed to process image id: cannot find tag")
	}
	url := fmt.Sprintf("/v2/%s/manifests/%s", parts[0], parts[1])

	s.responsesMap[url] = response{
		ContentType: "application/vnd.docker.distribution.manifest.list.v2+json",
		Body:        manifestResponse(img),
	}
	return nil
}

// Close shuts down the test server
func (s *TestServer) Close() {
	s.s.Close()
}

// LoadImagesFromFile adds the images specified in the JSON file provided to the server
func (s *TestServer) LoadImagesFromFile(file string) ([]*ImageData, error) {
	var allErrors error
	var referenceImages []*ImageData

	fh, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	dec := json.NewDecoder(fh)
	if err := dec.Decode(&referenceImages); err != nil {
		return nil, fmt.Errorf("failed to decode reference images: %w", err)
	}

	for _, img := range referenceImages {
		if err := s.AddImage(img); err != nil {
			allErrors = errors.Join(allErrors, err)
		}
	}
	return referenceImages, allErrors
}

// NewTestServer returns a new TestServer
func NewTestServer() (*TestServer, error) {
	testServer := &TestServer{responsesMap: make(map[string]response)}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "manifests") {
			resp, ok := testServer.responsesMap[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				_, err := w.Write([]byte(fmt.Sprintf("cannot find image %q", r.URL.Path)))
				if err != nil {
					log.Fatal(err)
				}
				return
			}
			w.Header().Set("Content-Type", resp.ContentType)
			w.WriteHeader(200)
			_, err := w.Write([]byte(resp.Body))
			if err != nil {
				log.Fatal(err)
			}
		} else if r.URL.Path == "/v2/" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(500)
		}
	}))
	testServer.s = s
	u, _ := url.Parse(s.URL)
	testServer.ServerURL = fmt.Sprintf("localhost:%s", u.Port())
	return testServer, nil
}

type response struct {
	Body        string
	ContentType string
}

func manifestResponse(img *ImageData) string {
	tmpl, err := template.New("test").Funcs(fns).Funcs(sprig.FuncMap()).Parse(`
	{{$listLen:= len .Digests}}
	{
        "schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json",
        "manifests":[
{{- range $i, $e := .Digests}}
{{- $archList := splitList "/" $e.Arch }}
{{- $os   := index $archList 0 }}
{{- $arch := index $archList 1 }}
            {
               "mediaType":"application/vnd.docker.distribution.manifest.v2+json",
               "size":430,"digest":"{{$e.Digest}}",
               "platform":{"architecture":"{{$arch}}","os":"{{$os}}"}
            }{{if not (isLast $i $listLen)}},{{end}}
{{- end}}
        ]
}`)
	if err != nil {
		log.Fatal(err)

	}
	b := &bytes.Buffer{}

	if err := tmpl.Execute(b, img); err != nil {
		log.Fatal(err)
	}

	_ = tmpl
	return strings.TrimSpace(b.String())
}
