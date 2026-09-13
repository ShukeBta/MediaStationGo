package service

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresBackupConnectionPreservesSettingsWithoutPasswordArguments(t *testing.T) {
	for _, dsn := range []string{
		"postgres://backup:sec%27ret%5C%20pass@localhost:15432/media?sslmode=disable&TimeZone=Asia%2FHong_Kong&application_name=Backup%20Client",
		`host=localhost port=15432 dbname=media user=backup password='sec\'ret\\ pass' sslmode=disable TimeZone=Asia/Hong_Kong application_name='Backup Client'`,
	} {
		conn, err := postgresBackupConnection(dsn)
		if err != nil {
			t.Fatal(err)
		}
		if conn.password != "sec'ret\\ pass" || strings.Contains(conn.dsn, "password") || strings.Contains(conn.dsn, "sec") {
			t.Fatalf("password was not separated from connection arguments")
		}
		var options string
		if strings.HasPrefix(conn.dsn, "postgres://") {
			u, _ := url.Parse(conn.dsn)
			options = u.Query().Get("options")
			if _, ok := u.User.Password(); ok {
				t.Fatal("URL retained a password")
			}
		} else {
			settings, err := parseBackupKeywordDSN(conn.dsn)
			if err != nil {
				t.Fatal(err)
			}
			options = settings["options"]
		}
		if options != "-c TimeZone=Asia/Hong_Kong" {
			t.Fatalf("native client session options = %q", options)
		}
		parsed, err := pgx.ParseConfig(conn.dsn)
		if err != nil || parsed.Database != "media" || parsed.Port != 15432 || parsed.RuntimeParams["application_name"] != "Backup Client" {
			t.Fatalf("connection settings were changed: %v", err)
		}
	}
}
