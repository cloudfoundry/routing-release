package migration

import (
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/models"
)

type V3UpdateTcpRouteMigration struct{}

var _ Migration = new(V3UpdateTcpRouteMigration)

func NewV3UpdateTcpRouteMigration() *V3UpdateTcpRouteMigration {
	return &V3UpdateTcpRouteMigration{}
}

func (v *V3UpdateTcpRouteMigration) Version() int {
	return 3
}

func (v *V3UpdateTcpRouteMigration) Run(sqlDB *db.SqlDB) error {
	return sqlDB.Client.Model(models.TcpRouteMapping{}).AutoMigrate(models.TcpRouteMapping{})
}
