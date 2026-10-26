package loadprofile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// WriteFactoryInputs creates a new private input directory. It never replaces
// an old run or records a native fact. The ephemeral CA signing key is discarded
// immediately; only its public certificate and the scoped server key persist.
func WriteFactoryInputs(directory string, plan FactoryPlan) error {
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("%w: absolute private fixture directory required", errCohort)
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	manifest, err := OIDCManifestForCohort(
		plan.Cohort,
		"https://issuer.load.test:8445",
		map[string]string{
			"load-public": "https://public.load.test:8443/auth/callback",
			"load-admin":  "https://admin.load.test:8444/auth/callback",
		},
	)
	if err != nil {
		return err
	}
	certificates, err := factoryCertificates(time.Now())
	if err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return fmt.Errorf("create NEW private fixture directory: %w", err)
	}
	// Failures preserve partial private setup artifacts for diagnosis instead
	// of deleting a directory a concurrent caller might now be inspecting.
	for name, content := range certificates {
		if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
			return fmt.Errorf("write private fixture certificate: %w", err)
		}
	}
	for name, value := range map[string]any{"factory-plan.json": plan, "oidc.json": manifest} {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("encode explicit fixture inputs: %w", err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), raw, 0o600); err != nil {
			return fmt.Errorf("write private fixture inputs: %w", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(directory, "domain-time"),
		[]byte(plan.SetupStart.Format(time.RFC3339Nano)),
		0o600,
	); err != nil {
		return fmt.Errorf("write setup-only historical clock: %w", err)
	}
	return nil
}

func factoryCertificates(now time.Time) (map[string][]byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private fixture CA: %w", err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate private fixture server key: %w", err)
	}
	caID, err := fixtureKeyID(caKey)
	if err != nil {
		return nil, err
	}
	serverID, err := fixtureKeyID(serverKey)
	if err != nil {
		return nil, err
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	caSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate fixture CA serial: %w", err)
	}
	serverSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate fixture server serial: %w", err)
	}
	ca := &x509.Certificate{
		SerialNumber: caSerial,
		Subject:      pkix.Name{CommonName: "Synthetic load123 fixture CA"},
		NotBefore: now.Add(
			-time.Minute,
		),
		NotAfter:              now.Add(48 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		SubjectKeyId:          caID,
		AuthorityKeyId:        caID,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("sign fixture CA: %w", err)
	}
	server := &x509.Certificate{
		SerialNumber:          serverSerial,
		Subject:               pkix.Name{CommonName: "Synthetic load123 frontend"},
		DNSNames:              []string{"public.load.test", "admin.load.test", "issuer.load.test"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(48 * time.Hour),
		SubjectKeyId:          serverID,
		AuthorityKeyId:        caID,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("sign fixture frontend: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return nil, fmt.Errorf("encode fixture server key: %w", err)
	}
	return map[string][]byte{
		"ca.crt":     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		"server.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		"server.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}, nil
}

func fixtureKeyID(key *ecdsa.PrivateKey) ([]byte, error) {
	encoded, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("encode fixture certificate key identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return digest[:20], nil
}
