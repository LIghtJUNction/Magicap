package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Root managers may create an app mount namespace after forking zygote and
// remove module mounts. Watch selected app UIDs while capture is active and
// repair only those namespaces before their first network requests.
func watchAppTrust(ctx context.Context, uids []int) {
	selected := make(map[int]bool, len(uids))
	for _, uid := range uids {
		selected[uid] = true
	}
	retryAfter := map[int]time.Time{}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(state, "trust", "enabled")); err != nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		out, err := exec.CommandContext(ctx, "/system/bin/ps", "-A", "-o", "UID,PID").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				if len(fields) != 2 {
					continue
				}
				uid, err := strconv.Atoi(fields[0])
				if err != nil || !selected[uid] {
					continue
				}
				pid, err := strconv.Atoi(fields[1])
				if err != nil || pid <= 1 {
					continue
				}
				mounts, err := os.ReadFile(filepath.Join("/proc", fields[1], "mountinfo"))
				if err != nil {
					continue
				}
				if strings.Contains(string(mounts), "/magicap/trust/cacerts /apex/com.android.conscrypt/cacerts ") {
					continue
				}
				if time.Now().Before(retryAfter[pid]) {
					continue
				}
				command := exec.CommandContext(ctx, "/system/bin/sh", filepath.Join(root, "trust-ca.sh"), "pid", fields[1])
				command.Stdout = os.Stderr
				command.Stderr = os.Stderr
				if err = command.Run(); err == nil {
					fmt.Fprintf(os.Stderr, "trust: CA visible to selected UID %d PID %d\n", uid, pid)
					delete(retryAfter, pid)
				} else {
					retryAfter[pid] = time.Now().Add(5 * time.Second)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
