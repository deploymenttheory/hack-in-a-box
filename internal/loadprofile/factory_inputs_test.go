package loadprofile

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFactoryCertificatesBindOnlyDeclaredNamesAndExpireOnRealWallTime(t *testing.T) {
	now := time.Now()
	files, err := factoryCertificates(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(files["server.crt"], files["server.key"]); err != nil {
		t.Fatal("private key does not match frontend certificate", err)
	}
	block, _ := pem.Decode(files["server.crt"])
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(files["ca.crt"]) || len(certificate.AuthorityKeyId) == 0 {
		t.Fatal("scoped CA chain/authority identity missing")
	}
	for _, name := range []string{"public.load.test", "admin.load.test", "issuer.load.test"} {
		if _, err := certificate.Verify(
			x509.VerifyOptions{DNSName: name, Roots: roots, CurrentTime: now},
		); err != nil {
			t.Fatal("declared external frontend rejected", name, err)
		}
	}
	for _, options := range []x509.VerifyOptions{
		{DNSName: "other.load.test", Roots: roots, CurrentTime: now},
		{DNSName: "public.load.test", Roots: roots, CurrentTime: now.Add(49 * time.Hour)},
	} {
		if _, err := certificate.Verify(options); err == nil {
			t.Fatal("undeclared name or expired fixture accepted")
		}
	}
	if len(files) != 3 || files["ca.key"] != nil {
		t.Fatal("CA signing key persisted beyond fixture generation")
	}
}

func TestFactoryInputsArePrivateAndCannotOverwriteHistoricalRun(t *testing.T) {
	plan, err := NewReleaseFactoryPlan(uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "new-run")
	if err := WriteFactoryInputs(directory, plan); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"factory-plan.json", "oidc.json", "ca.crt", "server.crt", "server.key", "domain-time"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("fixture input not owner-only", name, err)
		}
	}
	before, err := os.ReadFile(filepath.Join(directory, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFactoryInputs(directory, plan); err == nil {
		t.Fatal("existing historical run overwritten")
	}
	after, err := os.ReadFile(filepath.Join(directory, "server.crt"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed duplicate command replaced original fixture certificate", err)
	}
}
