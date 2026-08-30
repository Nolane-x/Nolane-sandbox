package agentruntime

import (
	"context"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/realm"
	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

type ServiceRequest struct {
	SessionID       realm.SessionID
	RealmRevision   uint64
	WorldID         world.ID
	LeaseGeneration uint64
	Name            string
	Protocol        realm.ServiceProtocol
	Port            uint16
	Ready           bool
}

type ServiceReceipt struct {
	ID                  realm.ServiceID       `json:"id"`
	WorldID             world.ID              `json:"world_id"`
	RealizationRevision uint64                `json:"realization_revision"`
	Protocol            realm.ServiceProtocol `json:"protocol"`
	Port                uint16                `json:"port"`
	Generation          uint64                `json:"generation"`
	Ready               bool                  `json:"ready"`
}

func (s *Service) RegisterService(ctx context.Context, req ServiceRequest) (ServiceReceipt,error) {
	if s==nil || req.WorldID=="" || req.LeaseGeneration==0 { return ServiceReceipt{},ErrInvalidRequest }
	sess,err:=s.validateSession(req.SessionID,req.RealmRevision);if err!=nil { return ServiceReceipt{},err }
	_,realization,err:=s.fabric.Handle(sess.RealmID,req.WorldID,req.LeaseGeneration);if err!=nil { return ServiceReceipt{},err }
	if err:=ctx.Err();err!=nil { return ServiceReceipt{},err }
	reg,err:=realm.NewServiceRegistry(s.store);if err!=nil { return ServiceReceipt{},err }
	rec,err:=reg.Register(realm.ServiceRequest{RealmID:sess.RealmID,WorldID:req.WorldID,RealizationRevision:realization,Name:req.Name,Protocol:req.Protocol,Port:req.Port,Ready:req.Ready});if err!=nil { return ServiceReceipt{},err }
	return ServiceReceipt{ID:rec.ID,WorldID:rec.WorldID,RealizationRevision:rec.RealizationRevision,Protocol:rec.Protocol,Port:rec.Port,Generation:rec.Generation,Ready:rec.Ready},nil
}
