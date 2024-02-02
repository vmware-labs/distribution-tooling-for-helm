package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestLogin(t *testing.T) {
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
	dt("auth", "logout", registryURI).AssertSuccessMatch(t, "logged out via")

	t.Cleanup(func() {
		if err := registry.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	})
}
