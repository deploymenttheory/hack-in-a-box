package recovery

import (
	"errors"
	"net/url"
	"os"
	"sort"
	"strings"
)

var ErrConnection = errors.New(
	"recovery PostgreSQL connection requires an explicit PostgreSQL URI and supported libpq options",
)

// libpq does not expand a URI supplied only as its PGDATABASE default. Map the
// current deployment URI to documented environment fields without putting a
// password in argv, inventing a DSN parser, or weakening an explicit TLS mode.
func postgresEnvironment(connection string) ([]string, error) {
	uri, err := url.Parse(connection)
	if err != nil || (uri.Scheme != "postgres" && uri.Scheme != "postgresql") ||
		uri.Fragment != "" ||
		strings.Contains(uri.Host, ",") {
		return nil, ErrConnection
	}
	fields := map[string]string{
		"PATH":       os.Getenv("PATH"),
		"LANG":       "C.UTF-8",
		"PGAPPNAME":  "hackbox_recovery",
		"PGHOST":     uri.Hostname(),
		"PGPORT":     uri.Port(),
		"PGDATABASE": strings.TrimPrefix(uri.Path, "/"),
	}
	if fields["PGPORT"] == "" {
		fields["PGPORT"] = "5432"
	}
	if uri.User != nil {
		fields["PGUSER"] = uri.User.Username()
		if password, present := uri.User.Password(); present {
			fields["PGPASSWORD"] = password
		}
	}
	supported := map[string]string{
		"host":                     "PGHOST",
		"hostaddr":                 "PGHOSTADDR",
		"port":                     "PGPORT",
		"user":                     "PGUSER",
		"password":                 "PGPASSWORD",
		"dbname":                   "PGDATABASE",
		"sslmode":                  "PGSSLMODE",
		"sslcert":                  "PGSSLCERT",
		"sslkey":                   "PGSSLKEY",
		"sslrootcert":              "PGSSLROOTCERT",
		"sslcrl":                   "PGSSLCRL",
		"sslcrldir":                "PGSSLCRLDIR",
		"sslcompression":           "PGSSLCOMPRESSION",
		"sslsni":                   "PGSSLSNI",
		"ssl_min_protocol_version": "PGSSLMINPROTOCOLVERSION",
		"ssl_max_protocol_version": "PGSSLMAXPROTOCOLVERSION",
		"sslnegotiation":           "PGSSLNEGOTIATION",
		"connect_timeout":          "PGCONNECT_TIMEOUT",
		"application_name":         "PGAPPNAME",
		"options":                  "PGOPTIONS",
		"client_encoding":          "PGCLIENTENCODING",
		"target_session_attrs":     "PGTARGETSESSIONATTRS",
		"passfile":                 "PGPASSFILE",
		"channel_binding":          "PGCHANNELBINDING",
		"require_auth":             "PGREQUIREAUTH",
		"gssencmode":               "PGGSSENCMODE",
	}
	query, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, ErrConnection
	}
	for key, values := range query {
		name, ok := supported[key]
		if !ok || len(values) != 1 {
			return nil, ErrConnection
		}
		fields[name] = values[0]
	}
	if fields["PGHOST"] == "" || fields["PGUSER"] == "" || fields["PGDATABASE"] == "" ||
		strings.Contains(fields["PGHOST"], ",") ||
		strings.Contains(fields["PGPORT"], ",") {
		return nil, ErrConnection
	}
	names := make([]string, 0, len(fields))
	for key := range fields {
		names = append(names, key)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, key := range names {
		if strings.ContainsRune(fields[key], 0) {
			return nil, ErrConnection
		}
		environment = append(environment, key+"="+fields[key])
	}
	return environment, nil
}
