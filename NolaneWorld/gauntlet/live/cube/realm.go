package cube

import (
	"context"
	"errors"
	"fmt"

	live "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live"
	realmproof "github.com/Nolane-x/Nolane-sandbox/NolaneWorld/gauntlet/live/realmproof"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
)

const (
	realmIngressPort   = 18080
	realmIngressPath   = "/nolane-live-v9-ingress"
	realmIngressMarker = "NOLANE_LIVE_V9_INGRESS"
)

func (b *box) ApplyRealmProfile(ctx context.Context, profile realm.NetworkProfile) error {
	if b == nil || b.client == nil || b.handle == "" {
		return live.ErrLiveUnavailable
	}
	return b.client.ApplyRealmProfile(ctx, b.handle, profile)
}

func (b *box) ProbePublicIngress(ctx context.Context) (realmproof.IngressObservation, error) {
	if b == nil || b.session == nil {
		return realmproof.IngressObservation{}, live.ErrLiveUnavailable
	}
	obs, err := b.session.RunCommand(ctx, realmIngressCanaryCommand())
	if err != nil {
		return realmproof.IngressObservation{}, err
	}
	switch obs.ExitCode {
	case 0:
		// Positive listener check passed inside the guest. The substrate-level
		// probe performs a second positive control through CubeProxy before it
		// accepts an unauthenticated 403 as denial evidence.
	case 125:
		return realmproof.IngressObservation{}, live.ErrProbeUnsupported
	default:
		return realmproof.IngressObservation{}, fmt.Errorf("realm ingress canary exit=%d", obs.ExitCode)
	}

	public, err := b.session.ProbeRestrictedPublicIngress(ctx, realmIngressPort, realmIngressPath, realmIngressMarker)
	if err != nil {
		if errors.Is(err, live.ErrProbeUnsupported) {
			return realmproof.IngressObservation{}, live.ErrProbeUnsupported
		}
		return realmproof.IngressObservation{}, err
	}
	if !public.CanaryReached {
		return realmproof.IngressObservation{}, live.ErrLiveUnavailable
	}
	return realmproof.IngressObservation{
		Denied: public.UnauthenticatedDenied,
		Marker: "external-restricted-proxy-denied",
	}, nil
}

func realmIngressCanaryCommand() string {
	dir := "/tmp/nolane-live-v9-ingress"
	log := "/tmp/nolane-live-v9-ingress.log"
	return "command -v python3 >/dev/null 2>&1 || exit 125; " +
		"mkdir -p " + shellQuote(dir) + " || exit 42; " +
		"printf %s " + shellQuote(realmIngressMarker) + " > " + shellQuote(dir+realmIngressPath) + " || exit 42; " +
		"(nohup python3 -m http.server 18080 --bind 0.0.0.0 --directory " + shellQuote(dir) + " >" + shellQuote(log) + " 2>&1 < /dev/null &) ; " +
		"for i in {1..30}; do (exec 3<>/dev/tcp/127.0.0.1/18080) >/dev/null 2>&1 && exit 0; sleep 0.1; done; exit 42"
}
