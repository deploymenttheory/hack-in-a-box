package recovery

import (
	"bytes"
	"crypto/tls"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// operatorConnection leaves the application's existing pgx configuration intact.
// Keyword-style deployments can supply an equivalent client URI separately;
// native parsing compares connection identity, credentials, TLS and startup state
// before any PostgreSQL subprocess is started. Pool-only tuning is not identity.
func operatorConnection(primary, equivalentURI string) (string, error) {
	if equivalentURI == "" {
		return primary, nil
	}
	source, err := pgxpool.ParseConfig(primary)
	if err != nil {
		return "", ErrConnection
	}
	client, err := pgxpool.ParseConfig(equivalentURI)
	if err != nil {
		return "", ErrConnection
	}
	a, b := source.ConnConfig.Config, client.ConnConfig.Config
	if a.Host != b.Host || a.Port != b.Port || a.Database != b.Database || a.User != b.User ||
		a.Password != b.Password ||
		a.ConnectTimeout != b.ConnectTimeout ||
		a.SSLNegotiation != b.SSLNegotiation ||
		a.ChannelBinding != b.ChannelBinding ||
		a.RequireAuth != b.RequireAuth ||
		a.KerberosSrvName != b.KerberosSrvName ||
		a.KerberosSpn != b.KerberosSpn ||
		!maps.Equal(a.RuntimeParams, b.RuntimeParams) ||
		!sameTLS(a.TLSConfig, b.TLSConfig) ||
		!sameFallbacks(a.Fallbacks, b.Fallbacks) {
		return "", ErrConnection
	}
	return equivalentURI, nil
}

func sameFallbacks(a, b []*pgconn.FallbackConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Host != b[i].Host || a[i].Port != b[i].Port ||
			!sameTLS(a[i].TLSConfig, b[i].TLSConfig) {
			return false
		}
	}
	return true
}

func sameTLS(a, b *tls.Config) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.InsecureSkipVerify != b.InsecureSkipVerify || a.ServerName != b.ServerName ||
		a.MinVersion != b.MinVersion ||
		a.MaxVersion != b.MaxVersion ||
		(a.VerifyPeerCertificate == nil) != (b.VerifyPeerCertificate == nil) ||
		(a.VerifyConnection == nil) != (b.VerifyConnection == nil) ||
		!slices.Equal(a.NextProtos, b.NextProtos) ||
		len(a.Certificates) != len(b.Certificates) {
		return false
	}
	if a.RootCAs == nil || b.RootCAs == nil {
		if a.RootCAs != nil || b.RootCAs != nil {
			return false
		}
	} else if !a.RootCAs.Equal(b.RootCAs) {
		return false
	}
	for i := range a.Certificates {
		if len(a.Certificates[i].Certificate) != len(b.Certificates[i].Certificate) {
			return false
		}
		for j := range a.Certificates[i].Certificate {
			if !bytes.Equal(a.Certificates[i].Certificate[j], b.Certificates[i].Certificate[j]) {
				return false
			}
		}
	}
	return true
}
