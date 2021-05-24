package migration

import (
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/models"
)

type V5SniHostnameMigration struct{}

var _ Migration = new(V5SniHostnameMigration)

func NewV5SniHostnameMigration() *V5SniHostnameMigration {
	return &V5SniHostnameMigration{}
}

func (v *V5SniHostnameMigration) Version() int {
	return 5
}

func (v *V5SniHostnameMigration) Run(sqlDB *db.SqlDB) error {
	_, err := sqlDB.Client.Model(&models.TcpRouteMapping{}).RemoveIndex("idx_tcp_route")
	if err != nil {
		return err
	}
	err = sqlDB.Client.AutoMigrate(&models.TcpRouteMapping{})
	if err != nil {
		return err
	}
	return err
}
