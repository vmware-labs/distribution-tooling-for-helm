package testutil

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/secure-systems-lab/go-securesystemslib/encrypted"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/sign"
	"github.com/sigstore/cosign/v2/cmd/cosign/cli/verify"

	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/cosign/v2/test"
)

// resolveImage gets a image and returns its resolved tag version
func resolveImage(image string) (string, error) {
	o := crane.GetOptions()

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
func CosignImage(url string, key string) error {
	url = strings.TrimPrefix(url, "oci://")
	// cosign complains if we sign a tag with
	// WARNING: Image reference 127.0.0.1/test:mytag uses a tag, not a digest, to identify the image to sign.
	image, err := resolveImage(url)
	if err != nil {
		return fmt.Errorf("failed to sign %q: %v", url, err)
	}
	return sign.SignCmd(&options.RootOptions{Timeout: options.DefaultTimeout, Verbose: false}, options.KeyOpts{KeyRef: key}, options.SignOptions{Upload: true}, []string{image})
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

// GenerateCosignCertificateFiles generates sample signing keys for usage with cosign
func GenerateCosignCertificateFiles(tmpDir string) (privFile, pubFile string, err error) {
	rootCert, rootKey, _ := test.GenerateRootCa()
	subCert, subKey, _ := test.GenerateSubordinateCa(rootCert, rootKey)
	leafCert, privKey, _ := test.GenerateLeafCert("subject", "oidc-issuer", subCert, subKey)
	pemRoot := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCert.Raw})
	pemSub := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: subCert.Raw})
	pemLeaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCert.Raw})

	x509EncodedPub, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode public key: %v", err)

	}
	x509Encoded, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode private key: %v", err)
	}

	password := []byte{}

	encBytes, err := encrypted.Encrypt(x509Encoded, password)
	if err != nil {
		return "", "", fmt.Errorf("failed to encrypt key: %v", err)
	}

	pubKeyBytes := pem.EncodeToMemory(&pem.Block{Bytes: x509EncodedPub, Type: "PUBLIC KEY"})

	// store in PEM format
	privBytes := pem.EncodeToMemory(&pem.Block{
		Bytes: encBytes,
		Type:  cosign.CosignPrivateKeyPemType,
	})

	tmpPrivFile, err := os.CreateTemp(tmpDir, "cosign_test_*.key")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp key file: %v", err)
	}

	defer tmpPrivFile.Close()
	_, err = tmpPrivFile.Write(privBytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to write key file: %v", err)
	}

	tmpPubFile, err := os.CreateTemp(tmpDir, "cosign_test_*.pub")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp pub key file: %v", err)
	}

	defer tmpPubFile.Close()
	_, err = tmpPubFile.Write(pubKeyBytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to write pub key file: %v", err)
	}

	tmpCertFile, err := os.CreateTemp(tmpDir, "cosign.crt")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp certificate file: %v", err)
	}
	defer tmpCertFile.Close()
	_, err = tmpCertFile.Write(pemLeaf)
	if err != nil {
		return "", "", fmt.Errorf("failed to write certificate file: %v", err)
	}

	tmpChainFile, err := os.CreateTemp(tmpDir, "cosign_chain.crt")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp chain file: %v", err)
	}
	defer tmpChainFile.Close()

	pemChain := pemSub
	pemChain = append(pemChain, pemRoot...)
	_, err = tmpChainFile.Write(pemChain)
	if err != nil {
		return "", "", fmt.Errorf("failed to write chain file: %v", err)
	}
	return tmpPrivFile.Name(), tmpPubFile.Name(), nil
}
