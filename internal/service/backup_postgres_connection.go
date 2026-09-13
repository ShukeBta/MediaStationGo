package service

import (
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type backupPostgresConnection struct{ dsn, password, serviceFile string }

// Keep the database password out of process arguments. pgx also accepts
// session parameters (such as TimeZone) directly in its DSN; libpq needs those
// under options instead. Transport/TLS parameters retain their original values.
func postgresBackupConnection(raw string) (backupPostgresConnection, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return backupPostgresConnection{}, errors.New("PostgreSQL backup requires database.dsn")
	}
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		// pgx parse errors include the original connection string.
		return backupPostgresConnection{}, errors.New("invalid PostgreSQL connection configuration")
	}
	conn := backupPostgresConnection{password: cfg.Password}
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		u, err := url.Parse(raw)
		if err != nil {
			return conn, errors.New("invalid PostgreSQL connection URL")
		}
		if u.User != nil {
			u.User = url.User(u.User.Username())
		}
		settings := map[string]string{}
		for key, values := range u.Query() {
			settings[key] = values[len(values)-1]
		}
		conn.serviceFile = settings["servicefile"]
		delete(settings, "servicefile")
		normalizePostgresBackupSettings(settings, cfg.RuntimeParams)
		query := url.Values{}
		for key, value := range settings {
			query.Set(key, value)
		}
		// libpq follows URI percent-encoding and treats '+' literally, unlike
		// application/x-www-form-urlencoded query parsers used by net/url.
		u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
		conn.dsn = u.String()
		return conn, nil
	}
	settings, err := parseBackupKeywordDSN(raw)
	if err != nil {
		return conn, err
	}
	conn.serviceFile = settings["servicefile"]
	delete(settings, "servicefile")
	normalizePostgresBackupSettings(settings, cfg.RuntimeParams)
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		value := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(settings[key])
		parts = append(parts, key+"='"+value+"'")
	}
	conn.dsn = strings.Join(parts, " ")
	return conn, nil
}

func normalizePostgresBackupSettings(settings, runtime map[string]string) {
	delete(settings, "password")
	for _, key := range []string{"default_query_exec_mode", "statement_cache_capacity", "description_cache_capacity"} {
		delete(settings, key)
	}
	if database, ok := settings["database"]; ok {
		settings["dbname"] = database
		delete(settings, "database")
	}
	keys := make([]string, 0, len(runtime))
	for key := range runtime {
		if _, ok := settings[key]; ok && key != "options" && key != "application_name" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.NewReplacer(`\`, `\\`, " ", `\ `, "\t", "\\\t", "\n", "\\\n", "\r", "\\\r").Replace(key + "=" + runtime[key])
		settings["options"] = strings.TrimSpace(settings["options"] + " -c " + value)
		delete(settings, key)
	}
}

func parseBackupKeywordDSN(raw string) (map[string]string, error) {
	settings := map[string]string{}
	for raw = strings.TrimSpace(raw); raw != ""; raw = strings.TrimSpace(raw) {
		equal := strings.IndexByte(raw, '=')
		if equal <= 0 {
			return nil, errors.New("invalid PostgreSQL keyword connection configuration")
		}
		key := strings.TrimSpace(raw[:equal])
		raw = strings.TrimLeft(raw[equal+1:], " \t\r\n")
		quoted := strings.HasPrefix(raw, "'")
		if quoted {
			raw = raw[1:]
		}
		var value strings.Builder
		for len(raw) > 0 {
			ch := raw[0]
			raw = raw[1:]
			if ch == '\\' && len(raw) > 0 {
				value.WriteByte(raw[0])
				raw = raw[1:]
			} else if (quoted && ch == '\'') || (!quoted && strings.ContainsRune(" \t\r\n", rune(ch))) {
				break
			} else {
				value.WriteByte(ch)
			}
		}
		settings[key] = value.String()
	}
	return settings, nil
}
