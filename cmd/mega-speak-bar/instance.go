package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// The file lock is owned by the process, so a crash cannot leave a stale owner.
// One user has one icon; subsequent launches request a view over a local socket.
func acquireInstance(dir, command string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, err
		}
		conn, dialErr := net.Dial("unix", instanceSocket(dir))
		if dialErr != nil {
			return nil, fmt.Errorf("Mega Speak is already starting; try opening it again shortly: %w", dialErr)
		}
		defer conn.Close()
		_, err = conn.Write([]byte(command + "\n"))
		return nil, err
	}
	return f, nil
}

func instanceSocket(dir string) string {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".cache/mega-speak-app")
	os.MkdirAll(root, 0700)
	path, _ := filepath.Abs(dir)
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(root, fmt.Sprintf("socket-%x.sock", sum[:8]))
}
