package maildelivery

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"
)

type (
	relayEvidence struct {
		recipients    []string
		body          []byte
		authenticated bool
	}
	localRelay struct {
		settings Settings
		roots    *x509.CertPool
		done     chan relayEvidence
		stop     context.CancelFunc
	}
)

func relayCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(76),
		Subject:               pkix.Name{CommonName: "Synthetic local SMTP"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool.AddCert(parsed)
	return cert, pool
}

func startRelay(t *testing.T, mode, transport string) localRelay {
	t.Helper()
	certificate, roots := relayCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); _ = listener.Close() })
	done := make(chan relayEvidence, 1)
	cfg := settings()
	cfg.Host = "127.0.0.1"
	cfg.Port = listener.Addr().(*net.TCPAddr).Port
	cfg.Mode = transport
	cfg.Timeout = 300 * time.Millisecond
	go func() {
		evidence := relayEvidence{}
		defer func() { done <- evidence }()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		raw := conn
		stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
		defer stop()
		protected := transport == "tls"
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS12,
		}
		if mode == "tls_timeout" {
			<-ctx.Done()
			return
		}
		if protected {
			conn = tls.Server(conn, tlsConfig)
		}
		reader := textproto.NewReader(bufio.NewReader(conn))
		writer := bufio.NewWriter(conn)
		reply := func(line string) bool {
			_, err := writer.WriteString(line + "\r\n")
			if err != nil {
				return false
			}
			return writer.Flush() == nil
		}
		if !reply("220 synthetic.local ESMTP") {
			return
		}
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
			switch verb {
			case "EHLO":
				response := "250-synthetic.local"
				if !protected && mode != "missing_tls" {
					response += "\r\n250-STARTTLS"
				}
				if mode == "missing_auth" {
					response += "\r\n250 SIZE 100000"
				} else {
					response += "\r\n250 AUTH PLAIN"
				}
				if !reply(response) {
					return
				}
			case "STARTTLS":
				if !reply("220 begin TLS") {
					return
				}
				if mode == "starttls_timeout" {
					<-ctx.Done()
					return
				}
				secure := tls.Server(conn, tlsConfig)
				if secure.HandshakeContext(ctx) != nil {
					return
				}
				conn = secure
				protected = true
				reader = textproto.NewReader(bufio.NewReader(conn))
				writer = bufio.NewWriter(conn)
			case "AUTH":
				if !protected {
					return
				}
				pieces := strings.Fields(line)
				if len(pieces) != 3 {
					return
				}
				decoded, err := base64.StdEncoding.DecodeString(pieces[2])
				if err != nil {
					return
				}
				if string(decoded) != "\x00"+cfg.Username+"\x00"+cfg.Password {
					if !reply("535 invalid authentication") {
						return
					}
					continue
				}
				evidence.authenticated = true
				if !reply("235 authenticated") {
					return
				}
			case "MAIL":
				if !evidence.authenticated {
					return
				}
				if !reply("250 sender accepted") {
					return
				}
			case "RCPT":
				evidence.recipients = append(evidence.recipients, line)
				if mode == "before_data_timeout" {
					<-ctx.Done()
					return
				}
				if mode == "rcpt_temporary" {
					if !reply("450 synthetic temporary rejection") {
						return
					}
					continue
				}
				if mode == "rcpt_permanent" {
					if !reply("550 synthetic permanent rejection") {
						return
					}
					continue
				}
				if !reply("250 recipient accepted") {
					return
				}
			case "DATA":
				if !reply("354 send message") {
					return
				}
				evidence.body, err = reader.ReadDotBytes()
				if err != nil {
					return
				}
				switch mode {
				case "uncertain_close":
					return
				case "after_data_timeout":
					<-ctx.Done()
					return
				case "data_temporary":
					if !reply("451 rejected before acceptance") {
						return
					}
				case "data_permanent":
					if !reply("554 rejected before acceptance") {
						return
					}
				default:
					if !reply("250 queued synthetic message") {
						return
					}
				}
			case "QUIT":
				if mode != "quit_failure" {
					_ = reply("221 goodbye")
				}
				return
			default:
				return
			}
		}
	}()
	return localRelay{settings: cfg, roots: roots, done: done, stop: cancel}
}

func TestSMTPAcceptanceRejectionAndUncertainty(t *testing.T) {
	cases := []struct {
		mode  string
		state State
	}{{"accepted", Accepted}, {"quit_failure", Accepted}, {"rcpt_temporary", Retryable}, {"rcpt_permanent", Permanent}, {"data_temporary", Retryable}, {"data_permanent", Permanent}, {"before_data_timeout", Retryable}, {"uncertain_close", Ambiguous}, {"after_data_timeout", Ambiguous}, {"missing_tls", Permanent}, {"missing_auth", Permanent}, {"starttls_timeout", Retryable}}
	for _, test := range cases {
		t.Run(test.mode, func(t *testing.T) {
			fixture := startRelay(t, test.mode, "starttls")
			sender, err := NewSMTP(fixture.settings)
			if err != nil {
				t.Fatal(err)
			}
			sender.roots = fixture.roots
			message := personal()
			result := sender.Send(t.Context(), message)
			if result.State != test.state || result.Code == "" {
				t.Fatalf("outcome %s/%s want%s", result.State, result.Code, test.state)
			}
			if strings.Contains(result.Code, "@") ||
				strings.Contains(result.Code, fixture.settings.Password) {
				t.Fatal("receipt exposes provider data")
			}
			fixture.stop()
			select {
			case evidence := <-fixture.done:
				if len(evidence.recipients) > 1 {
					t.Fatal("personal send combined recipients")
				}
				if len(evidence.body) > 0 &&
					(!bytes.Contains(evidence.body, []byte("To: "+message.Recipient)) || bytes.Contains(evidence.body, []byte("\nCc:")) || bytes.Contains(evidence.body, []byte("\nBcc:"))) {
					t.Fatal("unsafe wire recipients")
				}
				if result.State == Accepted &&
					(!evidence.authenticated || len(evidence.body) == 0 || len(evidence.recipients) != 1) {
					t.Fatal("accepted before authenticated DATA")
				}
			case <-time.After(time.Second):
				t.Fatal("local SMTP fixture did not finish")
			}
		})
	}
}

func TestSMTPImplicitTLSAndUntrustedCertificate(t *testing.T) {
	for _, trusted := range []bool{true, false} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			fixture := startRelay(t, "accepted", "tls")
			sender, err := NewSMTP(fixture.settings)
			if err != nil {
				t.Fatal(err)
			}
			if trusted {
				sender.roots = fixture.roots
			}
			result := sender.Send(t.Context(), personal())
			if trusted && result.State != Accepted {
				t.Fatal("trusted implicit TLS failed")
			}
			if !trusted && (result.State != Permanent || result.Code != "tls_verification_failed") {
				t.Fatal("untrusted TLS did not fail closed")
			}
			select {
			case evidence := <-fixture.done:
				if !trusted && (evidence.authenticated || len(evidence.body) > 0) {
					t.Fatal("sent credentials or content before TLS verification")
				}
			case <-time.After(time.Second):
				t.Fatal("fixture blocked")
			}
		})
	}
}
