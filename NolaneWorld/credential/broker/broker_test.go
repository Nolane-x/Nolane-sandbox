package broker

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/delegation"
)

func TestBrokerVaultUsesHandleOnlyAndLeasesSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requestCh := make(chan []byte, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		raw, err := readTestFrame(conn)
		if err != nil {
			serverErr <- err
			return
		}
		requestCh <- raw
		serverErr <- writeTestFrame(conn, []byte(`{"version":1,"secret_b64":"U1lOVEhFVElDLVY3LVNFQ1JFVA=="}`))
	}()

	vault, err := New(Config{SocketPath: path, ExpectedPeerUID: uint32(os.Getuid()), MaxSecretBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	var leased []byte
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := vault.Use(ctx, delegation.SecretHandle("kms/github/repo-a"), func(secret delegation.Secret) error {
		leased = append([]byte(nil), secret.Bytes()...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if string(leased) != "SYNTHETIC-V7-SECRET" {
		t.Fatalf("leased=%q", leased)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	raw := <-requestCh
	if string(raw) != `{"version":1,"handle":"kms/github/repo-a"}` {
		t.Fatalf("request=%s", raw)
	}
}

func TestBrokerVaultRejectsPeerUIDMismatchBeforeSendingHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	var readBytes atomic.Int64
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		buf := make([]byte, 1)
		if n, _ := conn.Read(buf); n > 0 {
			readBytes.Add(int64(n))
		}
	}()

	vault, err := New(Config{SocketPath: path, ExpectedPeerUID: uint32(os.Getuid()) + 1, MaxSecretBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = vault.Use(ctx, delegation.SecretHandle("kms/github/repo-a"), func(delegation.Secret) error {
		t.Fatal("callback invoked after peer mismatch")
		return nil
	})
	if err != ErrBrokerPeerMismatch {
		t.Fatalf("err=%v", err)
	}
	<-serverDone
	if readBytes.Load() != 0 {
		t.Fatal("handle bytes sent before peer identity check")
	}
}

func TestBrokerVaultRejectsInvalidConfiguration(t *testing.T) {
	cases := []Config{
		{},
		{SocketPath: "relative.sock", MaxSecretBytes: 1024},
		{SocketPath: "/tmp/../relative.sock", MaxSecretBytes: 1024},
		{SocketPath: "/tmp/secret.sock", MaxSecretBytes: -1},
		{SocketPath: "/tmp/secret.sock", MaxSecretBytes: 2 << 20},
	}
	for i, cfg := range cases {
		if v, err := New(cfg); err == nil || v != nil {
			t.Fatalf("case %d vault=%v err=%v", i, v, err)
		}
	}
}

func TestBrokerVaultDoesNotReturnBrokerControlledText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = readTestFrame(conn)
		_ = writeTestFrame(conn, []byte(`{"version":1,"error_code":"not_found","message":"SYNTHETIC-V7-SECRET"}`))
	}()
	vault, err := New(Config{SocketPath: path, ExpectedPeerUID: uint32(os.Getuid()), MaxSecretBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	err = vault.Use(context.Background(), delegation.SecretHandle("kms/github/repo-a"), func(delegation.Secret) error { return nil })
	if err == nil {
		t.Fatal("malformed broker response accepted")
	}
	if bytes.Contains([]byte(err.Error()), []byte("SYNTHETIC-V7-SECRET")) {
		t.Fatalf("broker-controlled text leaked: %v", err)
	}
}

func readTestFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	body := make([]byte, n)
	_, err := io.ReadFull(r, body)
	return body, err
}

func writeTestFrame(w io.Writer, body []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}
