package cube

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
	cubewire "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate/cube"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

const sentinelPath = "/tmp/nolane-live-v5-sentinel"

type Driver struct {
	client        *cubewire.Client
	fp            live.Fingerprint
	preflightHTTP *http.Client
}

func New(cfg cubewire.Config) (*Driver, error) {
	client, err := cubewire.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Driver{client: client, fp: live.Fingerprint{EndpointDigest: hash(strings.TrimRight(cfg.APIURL, "/")), TemplateDigest: hash(cfg.TemplateID)}, preflightHTTP: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func hash(s string) string                      { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (d *Driver) Fingerprint() live.Fingerprint { return d.fp }
func (d *Driver) Health(ctx context.Context) error {
	return d.client.Health(ctx)
}

func (d *Driver) Create(ctx context.Context, id world.ID) (live.Sandbox, error) {
	h, err := d.client.Create(ctx, id)
	if err != nil {
		return nil, err
	}
	session, err := d.client.ConnectGuest(ctx, h)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupErr := d.client.DestroyObserved(cleanupCtx, h, 250*time.Millisecond)
		if cleanupErr != nil {
			return nil, errors.Join(err, live.ErrCleanupFailed, cleanupErr)
		}
		return nil, err
	}
	return &box{client: d.client, handle: h, session: session}, nil
}

func (d *Driver) Preflight(ctx context.Context, t live.Target) error {
	switch t.Kind {
	case live.TargetHTTP:
		u, err := url.Parse(t.Address)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return live.ErrLiveUnavailable
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		resp, err := d.preflightHTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("target HTTP status %d", resp.StatusCode)
		}
		if t.Expect != "" && !strings.Contains(string(body), t.Expect) {
			return errors.New("target expectation mismatch")
		}
		return nil
	case live.TargetTCP:
		conn, err := net.DialTimeout("tcp", t.Address, 3*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		if t.Expect != "" {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, len(t.Expect))
			if _, err := io.ReadFull(conn, buf); err != nil {
				return err
			}
			if string(buf) != t.Expect {
				return errors.New("target banner mismatch")
			}
		}
		return nil
	case live.TargetUDP:
		if _, _, err := net.SplitHostPort(t.Address); err != nil {
			return err
		}
		payload := "NOLANE_LIVE_V5_UDP"
		conn, err := net.DialTimeout("udp", t.Address, 3*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.Write([]byte(payload)); err != nil {
			return err
		}
		buf := make([]byte, len(payload))
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}
		if string(buf[:n]) != payload {
			return errors.New("target UDP echo mismatch")
		}
		return nil
	case live.TargetDNS:
		if strings.TrimSpace(t.Address) == "" || strings.ContainsAny(t.Address, " /:@") {
			return live.ErrLiveUnavailable
		}
		ips, err := net.DefaultResolver.LookupHost(ctx, t.Address)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("target DNS empty")
		}
		if t.Expect != "" {
			for _, ip := range ips {
				if ip == t.Expect {
					return nil
				}
			}
			return errors.New("target DNS expectation mismatch")
		}
		return nil
	default:
		return live.ErrLiveUnavailable
	}
}

type box struct {
	client  *cubewire.Client
	handle  substrate.Handle
	session *cubewire.GuestSession
}

func (b *box) Digest() string { return b.session.SandboxDigest() }
func (b *box) Canary(ctx context.Context) error {
	obs, err := b.session.RunCanary(ctx)
	if err != nil {
		return err
	}
	if obs.ExitCode != 0 || obs.Stdout != "NOLANE_LIVE_V5_CANARY" || obs.Stderr != "" {
		return errors.New("live canary mismatch")
	}
	return nil
}
func (b *box) PutSentinel(ctx context.Context, value string) error {
	if value != "A" && value != "B" {
		return errors.New("unsupported sentinel")
	}
	obs, err := b.session.RunCommand(ctx, "printf %s "+shellQuote(value)+" > "+sentinelPath)
	if err != nil {
		return err
	}
	if obs.ExitCode != 0 {
		return errors.New("write sentinel failed")
	}
	return nil
}
func (b *box) ReadSentinel(ctx context.Context) (string, error) {
	obs, err := b.session.RunCommand(ctx, "cat "+sentinelPath)
	if err != nil {
		return "", err
	}
	if obs.ExitCode != 0 {
		return "", errors.New("read sentinel failed")
	}
	return strings.TrimSpace(obs.Stdout), nil
}
func (b *box) Snapshot(ctx context.Context) (substrate.Snapshot, error) {
	return b.client.Snapshot(ctx, b.handle)
}
func (b *box) Rollback(ctx context.Context, s substrate.Snapshot) error {
	if err := b.client.Rollback(ctx, b.handle, s); err != nil {
		return err
	}
	session, err := b.client.ConnectGuest(ctx, b.handle)
	if err != nil {
		return err
	}
	b.session = session
	return nil
}
func (b *box) DestroyObserved(_ context.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return b.client.DestroyObserved(ctx, b.handle, 250*time.Millisecond)
}
func (b *box) ProbeEgress(ctx context.Context, t live.Target) (live.EgressObservation, error) {
	cmd, err := probeCommand(t)
	if err != nil {
		return live.EgressObservation{}, live.ErrProbeUnsupported
	}
	obs, err := b.session.RunCommand(ctx, cmd)
	if err != nil {
		return live.EgressObservation{}, err
	}
	switch obs.ExitCode {
	case 0:
		return live.EgressObservation{Reached: true, Marker: "guest-probe-exercised"}, nil
	case 42:
		return live.EgressObservation{Reached: false, Marker: "guest-probe-exercised"}, nil
	case 125:
		return live.EgressObservation{Marker: "guest-probe-unsupported"}, live.ErrProbeUnsupported
	default:
		return live.EgressObservation{Marker: "guest-probe-error"}, fmt.Errorf("guest probe exit=%d", obs.ExitCode)
	}
}

func probeCommand(t live.Target) (string, error) {
	switch t.Kind {
	case live.TargetHTTP:
		u, err := url.Parse(t.Address)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return "", live.ErrProbeUnsupported
		}
		return "command -v curl >/dev/null 2>&1 || exit 125; code=$(curl -sS -o /dev/null -w '%{http_code}' --connect-timeout 2 --max-time 4 " + shellQuote(u.String()) + " 2>/dev/null); rc=$?; if [ $rc -eq 0 ] || [ \"$code\" != \"000\" ]; then exit 0; else exit 42; fi", nil
	case live.TargetTCP:
		host, port, err := net.SplitHostPort(t.Address)
		if err != nil || host == "" || port == "" || !safeTCPHost(host) {
			return "", live.ErrProbeUnsupported
		}
		inner := "exec 3<>/dev/tcp/" + host + "/" + port
		return "command -v timeout >/dev/null 2>&1 || exit 125; timeout 4 bash -c " + shellQuote(inner) + " >/dev/null 2>&1; rc=$?; [ $rc -eq 0 ] && exit 0; exit 42", nil
	case live.TargetUDP:
		host, portRaw, err := net.SplitHostPort(t.Address)
		if err != nil || host == "" {
			return "", live.ErrProbeUnsupported
		}
		port, err := strconv.Atoi(portRaw)
		if err != nil || port < 1 || port > 65535 {
			return "", live.ErrProbeUnsupported
		}
		payload := "NOLANE_LIVE_V5_UDP"
		enc := base64.StdEncoding.EncodeToString([]byte(payload))
		py := fmt.Sprintf("import socket,base64,sys;s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.settimeout(3);p=base64.b64decode(%q);s.sendto(p,(%q,%d));exec(\"try:\\n d,_=s.recvfrom(65535)\\n sys.exit(0 if d==p else 2)\\nexcept Exception:\\n sys.exit(42)\")", enc, host, port)
		return "command -v python3 >/dev/null 2>&1 || exit 125; python3 -c " + shellQuote(py), nil
	case live.TargetDNS:
		if strings.TrimSpace(t.Address) == "" || strings.ContainsAny(t.Address, " /:@") {
			return "", live.ErrProbeUnsupported
		}
		return "command -v getent >/dev/null 2>&1 || exit 125; getent ahosts " + shellQuote(t.Address) + " >/dev/null 2>&1; rc=$?; [ $rc -eq 0 ] && exit 0; exit 42", nil
	default:
		return "", live.ErrProbeUnsupported
	}
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

var safeTCPHostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

func safeTCPHost(host string) bool {
	return safeTCPHostPattern.MatchString(host) && !strings.Contains(host, "..")
}
