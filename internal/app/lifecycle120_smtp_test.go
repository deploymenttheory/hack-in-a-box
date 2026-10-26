package app

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The relay uses real TLS and SMTP AUTH. Its ephemeral CA is trusted through
// Go's normal SSL_CERT_FILE configuration in these test-only containers; no
// certificate verification is disabled in the native SMTP owner.
type lifecycleRelay struct {
	listener net.Listener
	mu       sync.Mutex
	faults   map[string]string
	messages []lifecycleMail
}

type lifecycleMail struct {
	Recipient string `json:"recipient"`
	Digest    string `json:"sha256"`
	Body      string `json:"body"`
	Accepted  bool   `json:"accepted"`
}

func newLifecycleRelay(ctx context.Context, directory string) (*lifecycleRelay, error) {
	certificate, ca, err := lifecycleRelayCertificate(time.Now())
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "ca.pem"), ca, 0o600); err != nil {
		return nil, err
	}
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", "0.0.0.0:2525")
	if err != nil {
		return nil, err
	}
	relay := &lifecycleRelay{listener: listener, faults: map[string]string{}, messages: []lifecycleMail{}}
	secure := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	slots := make(chan struct{}, 8)
	go func() {
		for {
			connection, err := secure.Accept()
			if err != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() {
					defer func() { <-slots }()
					relay.serve(ctx, connection)
				}()
			default:
				_ = connection.Close()
			}
		}
	}()
	return relay, nil
}

func lifecycleRelayCertificate(now time.Time) (tls.Certificate, []byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Isolated synthetic lifecycle CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	server := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "provider"},
		DNSNames: []string{"provider"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, server, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), nil
}

func (s *lifecycleRelay) serve(ctx context.Context, connection net.Conn) {
	defer func() { _ = connection.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	if connection.SetDeadline(time.Now().Add(30*time.Second)) != nil {
		return
	}
	reader := textproto.NewReader(bufio.NewReader(connection))
	writer := bufio.NewWriter(connection)
	reply := func(value string) bool {
		if _, err := writer.WriteString(value + "\r\n"); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !reply("220 provider ESMTP synthetic lifecycle") {
		return
	}
	authenticated, sender := false, false
	recipient, fault := "", ""
	for {
		line, err := reader.ReadLine()
		if err != nil || len(line) > 2048 {
			return
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			return
		}
		switch strings.ToUpper(parts[0]) {
		case "EHLO":
			if !reply("250-provider\r\n250 AUTH PLAIN") {
				return
			}
		case "AUTH":
			if len(parts) != 3 || parts[1] != "PLAIN" {
				return
			}
			value, err := base64.StdEncoding.DecodeString(parts[2])
			if err != nil || string(value) != "\x00lifecycle\x00synthetic-relay-secret" {
				_ = reply("535 synthetic authentication rejected")
				return
			}
			authenticated = true
			if !reply("235 authenticated") {
				return
			}
		case "MAIL":
			if !authenticated || !strings.HasPrefix(line, "MAIL FROM:<lifecycle@example.test>") {
				return
			}
			sender, recipient = true, ""
			if !reply("250 sender accepted") {
				return
			}
		case "RCPT":
			if !sender || recipient != "" || !strings.HasPrefix(line, "RCPT TO:<") || !strings.HasSuffix(line, "@example.test>") {
				return
			}
			recipient = strings.TrimSuffix(strings.TrimPrefix(line, "RCPT TO:<"), ">")
			s.mu.Lock()
			fault = s.faults[recipient]
			s.mu.Unlock()
			if fault == "temporary" {
				_ = reply("451 synthetic temporary rejection")
				return
			}
			if fault == "permanent" {
				_ = reply("550 synthetic permanent rejection")
				return
			}
			if !reply("250 recipient accepted") {
				return
			}
		case "DATA":
			if recipient == "" || !reply("354 send message") {
				return
			}
			data, err := io.ReadAll(io.LimitReader(reader.DotReader(), (2<<20)+1))
			if err != nil || len(data) > 2<<20 {
				return
			}
			digest := sha256.Sum256(data)
			s.mu.Lock()
			if len(s.messages) >= 1000 {
				s.mu.Unlock()
				_ = reply("452 synthetic mailbox capacity")
				return
			}
			s.messages = append(s.messages, lifecycleMail{Recipient: recipient, Digest: hex.EncodeToString(digest[:]), Body: string(data), Accepted: fault != "ambiguous"})
			s.mu.Unlock()
			if fault == "ambiguous" || !reply("250 synthetic relay accepted DATA") {
				return
			}
		case "QUIT":
			_ = reply("221 goodbye")
			return
		default:
			return
		}
	}
}

// Raw synthetic mailbox content, including confirmation links, is accessible
// only through the owner-private fixture socket. It is never a release receipt
// or log. Exported evidence must use only digest/count and bounded assertions.
func (s *lifecycleRelay) control(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/mail/messages" {
		s.mu.Lock()
		defer s.mu.Unlock()
		lifecycleJSON(w, map[string]any{"items": s.messages})
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/mail/fault" {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Recipient string `json:"recipient"`
		Mode      string `json:"mode"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil || !strings.HasSuffix(input.Recipient, "@example.test") ||
		(input.Mode != "" && input.Mode != "temporary" && input.Mode != "permanent" && input.Mode != "ambiguous") {
		http.Error(w, "invalid synthetic mailbox fault", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.faults[input.Recipient] = input.Mode
	s.mu.Unlock()
	lifecycleJSON(w, map[string]bool{"configured": true})
}

func (s *lifecycleRelay) close() error {
	if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
