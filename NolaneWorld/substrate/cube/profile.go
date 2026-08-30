package cube

import (
	"context"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/membrane"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/substrate"
)

// ApplyRealmProfile projects a host-owned Realm network profile onto Cube.
// Realm profiles never grant raw Internet or public ingress directly; R1/R2
// reach public resources only through the governed Reality gateway.
func (c *Client) ApplyRealmProfile(ctx context.Context, handle substrate.Handle, profile realm.NetworkProfile) error {
	plan, err := membrane.Plan(profile)
	if err != nil {
		return err
	}
	allowInternet := plan.RawPublicInternetAllowed
	allowPublicTraffic := plan.PublicInboundAllowed
	return c.UpdateNetwork(ctx, handle, NetworkPolicy{
		AllowInternetAccess: &allowInternet,
		AllowPublicTraffic:  &allowPublicTraffic,
	})
}
