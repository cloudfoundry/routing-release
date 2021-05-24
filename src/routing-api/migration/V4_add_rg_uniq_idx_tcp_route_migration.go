package migration

import (
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/models"
)

type V4AddRgUniqIdxTCPRoute struct{}

var _ Migration = new(V4AddRgUniqIdxTCPRoute)

func NewV4AddRgUniqIdxTCPRouteMigration() *V4AddRgUniqIdxTCPRoute {
	return &V4AddRgUniqIdxTCPRoute{}
}

func (v *V4AddRgUniqIdxTCPRoute) Version() int {
	return 4
}

func (v *V4AddRgUniqIdxTCPRoute) Run(sqlDB *db.SqlDB) error {
	_, err := sqlDB.Client.Model(&models.TcpRouteMapping{}).RemoveIndex("idx_tcp_route")
	if err != nil {
		return err
	}
	_, err = sqlDB.Client.Model(&models.TcpRouteMapping{}).AddUniqueIndex("idx_tcp_route", "router_group_guid", "host_port", "host_ip", "external_port")
	return err
}
