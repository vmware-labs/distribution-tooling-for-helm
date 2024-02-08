package main

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/stretchr/testify/require"
	"github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"helm.sh/helm/v3/pkg/repo/repotest"
)

func TestLoginLogout(t *testing.T) {
	srv, err := repotest.NewTempServerWithCleanup(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	ociSrv, err := testutil.NewOCIServer(t, srv.Root())
	if err != nil {
		t.Fatal(err)
	}
	go ociSrv.ListenAndServe()

	t.Run("can't get catalog without login", func(t *testing.T) {
		_, err := crane.Catalog(ociSrv.RegistryURL)
		require.ErrorContains(t, err, "UNAUTHORIZED")
	})

	t.Run("can get catalog after login", func(t *testing.T) {
		dt("auth", "login", ociSrv.RegistryURL, "-u", "username", "-p", "password").AssertSuccessMatch(t, "logged in via")
		_, err := crane.Catalog(ociSrv.RegistryURL)
		require.NoError(t, err)

		dt("auth", "logout", ociSrv.RegistryURL).AssertSuccessMatch(t, "logged out via")
		_, err = crane.Catalog(ociSrv.RegistryURL)
		require.ErrorContains(t, err, "UNAUTHORIZED")
	})
}
