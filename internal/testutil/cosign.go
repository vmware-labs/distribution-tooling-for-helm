package testutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/go-tuf/encrypted"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/sign"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/verify"
	"github.com/sigstore/cosign/v2/pkg/cosign"
)

// resolveImage gets a image and returns its resolved tag version
func resolveImage(image string, auth authn.Authenticator) (string, error) {
	o := crane.GetOptions(crane.WithAuth(auth))

	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	switch v := ref.(type) {
	case name.Tag:
		desc, err := remote.Get(ref, o.Remote...)
		if err != nil {
			return "", fmt.Errorf("failed to get remote descriptor: %w", err)
		}
		return fmt.Sprintf("%s@%s", ref.Context().Name(), desc.Digest), nil
	case name.Digest:
		// We already got a digest
		return image, nil
	default:
		return "", fmt.Errorf("unsupported reference type %T", v)
	}
}

// CosignImage signs a remote artifact with the provided key
func CosignImage(url string, key string, auth authn.Authenticator) error {
	o := crane.GetOptions(crane.WithAuth(auth))
	url = strings.TrimPrefix(url, "oci://")
	// cosign complains if we sign a tag with
	// WARNING: Image reference 127.0.0.1/test:mytag uses a tag, not a digest, to identify the image to sign.
	image, err := resolveImage(url, auth)
	if err != nil {
		return fmt.Errorf("failed to sign %q: %v", url, err)
	}
	return sign.SignCmd(&options.RootOptions{Timeout: options.DefaultTimeout, Verbose: false}, options.KeyOpts{KeyRef: key}, options.SignOptions{Upload: true, Registry: options.RegistryOptions{RegistryClientOpts: o.Remote}}, []string{image})
}

// CosignVerifyImage verifies a remote artifact signature with the provided key
func CosignVerifyImage(url string, key string) error {
	url = strings.TrimPrefix(url, "oci://")

	v := &verify.VerifyCommand{
		KeyRef:     key,
		IgnoreTlog: true,
	}
	v.NameOptions = append(v.NameOptions, name.Insecure)
	ctx := context.Background()
	return v.Exec(ctx, []string{url})
}

func writeTempFile(dir, name string, data []byte) (*os.File, error) {
	fh, err := os.CreateTemp(dir, name)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %v", err)
	}
	defer fh.Close()
	if _, err := fh.Write(data); err != nil {
		return nil, err
	}
	return fh, nil
}

// GenerateCosignCertificateFiles generates sample signing keys for usage with cosign
func GenerateCosignCertificateFiles(tmpDir string) (privFile, pubFile string, err error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	encodedPub, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode public key: %v", err)

	}
	encodedPriv, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode private key: %v", err)
	}

	password := []byte{}

	encryptedPrivBytes, err := encrypted.Encrypt(encodedPriv, password)
	if err != nil {
		return "", "", fmt.Errorf("failed to encrypt key: %v", err)
	}

	privKeyFile, err := writeTempFile(tmpDir, "cosign_test_*.key", pem.EncodeToMemory(&pem.Block{
		Bytes: encryptedPrivBytes,
		Type:  cosign.CosignPrivateKeyPemType,
	}))
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp key file: %v", err)
	}

	pubKeyFile, err := writeTempFile(tmpDir, "cosign_test_*.pub", pem.EncodeToMemory(&pem.Block{
		Bytes: encodedPub,
		Type:  "PUBLIC KEY",
	}))

	if err != nil {
		return "", "", fmt.Errorf("failed to write pub key file: %v", err)
	}

	return privKeyFile.Name(), pubKeyFile.Name(), nil

}
