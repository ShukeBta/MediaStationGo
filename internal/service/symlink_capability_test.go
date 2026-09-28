package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const symlinkProbeEnv = "MEDIASTATION_TEST_SYMLINK_PROBE_DIR"
const symlinkProbeReady = "symlink-capability-ready"
const symlinkProbeUnavailableExit = 77
const symlinkProbeTimeout = 40 * time.Second

var testSymlinkCapability struct {
	once        sync.Once
	unavailable string
	err         error
}

// A Windows filesystem or host policy can reject or stall CreateSymbolicLink.
// Probe that capability separately; errors from production code still fail its
// tests once the probe succeeds. A child process bounds a stalled system call.
func requireTestSymlinkCapability(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	testSymlinkCapability.once.Do(func() {
		testSymlinkCapability.unavailable, testSymlinkCapability.err = probeTestSymlinkCapability(t.TempDir())
	})
	if testSymlinkCapability.err != nil {
		t.Fatalf("symbolic-link capability probe failed: %v", testSymlinkCapability.err)
	}
	if testSymlinkCapability.unavailable != "" {
		t.Skipf("Windows symbolic links unavailable: %s", testSymlinkCapability.unavailable)
	}
}

func probeTestSymlinkCapability(dir string) (unavailable string, err error) {
	if err := os.WriteFile(filepath.Join(dir, "source"), []byte("probe"), 0o600); err != nil {
		return "", err
	}
	// Some Windows hosts return a policy denial after about 30 seconds. Allow
	// that call to finish normally before starting other external-tool tests.
	ctx, cancel := context.WithTimeout(context.Background(), symlinkProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSymlinkCapabilityProbeProcess$")
	cmd.Env = append(os.Environ(), symlinkProbeEnv+"="+dir)
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.CombinedOutput()
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if bytes.Contains(output, []byte(symlinkProbeReady)) {
			return fmt.Sprintf("os.Symlink did not return within %s", symlinkProbeTimeout), nil
		}
		return "", fmt.Errorf("probe process did not reach os.Symlink: %s", output)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == symlinkProbeUnavailableExit {
		return strings.TrimSpace(strings.TrimPrefix(string(output), symlinkProbeReady+"\n")), nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, output)
	}
	return "", nil
}

// Invoked only in the isolated subprocess above; normal suite execution is a no-op.
func TestSymlinkCapabilityProbeProcess(t *testing.T) {
	dir := os.Getenv(symlinkProbeEnv)
	if dir == "" {
		return
	}
	fmt.Fprintln(os.Stdout, symlinkProbeReady)
	err := os.Symlink(filepath.Join(dir, "source"), filepath.Join(dir, "link"))
	if err != nil {
		fmt.Fprintln(os.Stdout, err)
		if errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "required privilege") {
			os.Exit(symlinkProbeUnavailableExit)
		}
		os.Exit(1)
	}
	os.Exit(0)
}
