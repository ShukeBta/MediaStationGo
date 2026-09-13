package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type backupCommandRunner func(context.Context, string, []string, []string) (string, error)

func (b *BackupService) createPostgres(ctx context.Context, dst string) error {
	return b.runPostgresTool(ctx, "pg_dump", []string{"--format=custom", "--no-owner", "--no-privileges", "--file=" + dst})
}

func (b *BackupService) restorePostgres(ctx context.Context, src string) error {
	if err := b.runPostgresTool(ctx, "pg_restore", []string{"--list", src}); err != nil {
		return err
	}
	return b.runPostgresTool(ctx, "pg_restore", []string{
		"--clean", "--if-exists", "--no-owner", "--no-privileges",
		"--single-transaction", "--exit-on-error", src,
	})
}

func (b *BackupService) runPostgresTool(ctx context.Context, program string, args []string) error {
	conn, err := postgresBackupConnection(b.cfg.Database.DSN)
	if err != nil {
		return err
	}
	args = append([]string{"--no-password", "--dbname=" + conn.dsn}, args...)
	env := append(os.Environ(), "PGPASSWORD="+conn.password, "PGCONNECT_TIMEOUT=10")
	if conn.serviceFile != "" {
		env = append(env, "PGSERVICEFILE="+conn.serviceFile)
	}
	runner := b.runCommand
	if runner == nil {
		runner = runBackupCommand
	}
	output, err := runner(ctx, program, args, env)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	detail := strings.TrimSpace(output)
	if detail == "" {
		detail = err.Error()
	}
	for _, secret := range []string{b.cfg.Database.DSN, conn.password} {
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[redacted]")
		}
	}
	return fmt.Errorf("%s failed: %s", program, detail)
}

func runBackupCommand(ctx context.Context, program string, args, env []string) (string, error) {
	path, err := exec.LookPath(program)
	if err != nil {
		return "", fmt.Errorf("%s is unavailable; install PostgreSQL client tools on PATH matching the server major version", program)
	}
	cmd := exec.CommandContext(ctx, path, args...) // #nosec G204 -- program is a fixed pg_dump/pg_restore command, with no shell.
	cmd.Env = env
	stderr := &backupCommandOutput{}
	cmd.Stderr = stderr
	err = cmd.Run()
	return stderr.String(), err
}

// A failed restore can emit many SQL errors. Retain the first diagnostics
// without allowing unbounded buffering of the client's stderr.
type backupCommandOutput struct{ strings.Builder }

func (b *backupCommandOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Builder.Write(p)
	}
	return n, nil
}
