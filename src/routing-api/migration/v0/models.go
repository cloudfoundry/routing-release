package v0

import "time"

type Model struct {
	Guid      string    `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

type ModificationTag struct {
	Guid  string `gorm:"column:modification_guid" json:"guid"`
	Index uint32 `gorm:"column:modification_index" json:"index"`
}

type TcpRouteMapping struct {
	Model
	ExpiresAt time.Time `json:"-"`
	TcpMappingEntity
}

func (TcpRouteMapping) TableName() string {
	return "tcp_routes"
}

type TcpMappingEntity struct {
	RouterGroupGuid string `json:"router_group_guid"`
	HostPort        uint16 `gorm:"not null; unique_index:idx_tcp_route; type:int" json:"backend_port"`
	HostIP          string `gorm:"not null; unique_index:idx_tcp_route" json:"backend_ip"`
	ExternalPort    uint16 `gorm:"not null; unique_index:idx_tcp_route; type: int" json:"port"`
	ModificationTag `json:"modification_tag"`
	TTL             *int `json:"ttl,omitempty"`
}

type Route struct {
	Model
	ExpiresAt time.Time `json:"-"`
	RouteEntity
}

type RouteEntity struct {
	Route           string `gorm:"not null; unique_index:idx_route" json:"route"`
	Port            uint16 `gorm:"not null; unique_index:idx_route" json:"port"`
	IP              string `gorm:"not null; unique_index:idx_route" json:"ip"`
	TTL             *int   `json:"ttl"`
	LogGuid         string `json:"log_guid"`
	RouteServiceUrl string `gorm:"not null; unique_index:idx_route" json:"route_service_url,omitempty"`
	ModificationTag `json:"modification_tag"`
}

type RouterGroupDB struct {
	Model
	Name            string
	Type            string
	ReservablePorts string
}

func (RouterGroupDB) TableName() string {
	return "router_groups"
}

type RouterGroup struct {
	Model
	Guid            string `json:"guid"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	ReservablePorts string `json:"reservable_ports" yaml:"reservable_ports"`
}
