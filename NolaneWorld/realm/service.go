package realm

import (
	"errors"
	"regexp"
	"strings"

	"github.com/Nolane-x/Nolane-sandbox/NolaneWorld/world"
)

var ErrInvalidServiceRegistry = errors.New("realm: invalid service registry")
var serviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}[A-Za-z0-9]$|^[A-Za-z0-9]$`)

type ServiceRequest struct {
	RealmID             ID
	WorldID             world.ID
	RealizationRevision uint64
	Name                string
	Protocol            ServiceProtocol
	Port                uint16
	Ready               bool
}

type ServiceRegistry struct { store Store }

func NewServiceRegistry(store Store) (*ServiceRegistry,error) {
	if store==nil { return nil,ErrInvalidServiceRegistry }
	return &ServiceRegistry{store:store},nil
}

func (r *ServiceRegistry) Register(req ServiceRequest) (ServiceRecord,error) {
	if r==nil || r.store==nil || !validRealmID(req.RealmID) || req.WorldID=="" || req.RealizationRevision==0 || !validServiceName(req.Name) || !validServiceProtocol(req.Protocol) || req.Port==0 {
		return ServiceRecord{},ErrInvalidService
	}
	wr,ok:=r.store.World(req.RealmID,req.WorldID)
	if !ok || wr.Phase==WorldTerminal || wr.RealizationRevision!=req.RealizationRevision { return ServiceRecord{},ErrInvalidService }
	id:=serviceID(req.RealmID,req.Name)
	generation:=uint64(1)
	if old,ok:=r.store.Service(id);ok {
		if old.RealmID!=req.RealmID || old.WorldID!=req.WorldID { return ServiceRecord{},ErrInvalidService }
		generation=old.Generation+1
	}
	rec:=ServiceRecord{ID:id,RealmID:req.RealmID,WorldID:req.WorldID,RealizationRevision:req.RealizationRevision,Protocol:req.Protocol,Port:req.Port,Generation:generation,Ready:req.Ready}
	if err:=r.store.PutService(rec);err!=nil { return ServiceRecord{},err }
	return rec,nil
}

func (r *ServiceRegistry) Current(id ServiceID) (ServiceRecord,bool) {
	if r==nil || r.store==nil { return ServiceRecord{},false }
	rec,ok:=r.store.Service(id);if !ok { return ServiceRecord{},false }
	wr,ok:=r.store.World(rec.RealmID,rec.WorldID)
	if !ok || wr.Phase==WorldTerminal || wr.RealizationRevision!=rec.RealizationRevision || !rec.Ready { return ServiceRecord{},false }
	return rec,true
}

func serviceID(realmID ID,name string) ServiceID {
	opaque:=strings.TrimPrefix(string(realmID),"realm://")
	return ServiceID("service://"+opaque+"/"+name)
}
func validServiceName(name string) bool { return serviceName.MatchString(name) }
func validServiceProtocol(p ServiceProtocol) bool { switch p { case ServiceTCP,ServiceUDP,ServiceHTTP:return true;default:return false } }
