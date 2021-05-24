package migration

import (
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/models"
)

type V2UpdateRgMigration struct{}

var _ Migration = new(V2UpdateRgMigration)

func NewV2UpdateRgMigration() *V2UpdateRgMigration {
	return &V2UpdateRgMigration{}
}

func (v *V2UpdateRgMigration) Version() int {
	return 2
}

func (v *V2UpdateRgMigration) Run(sqlDB *db.SqlDB) error {
	_, err := sqlDB.Client.Model(&models.RouterGroup{}).AddUniqueIndex("idx_rg_name", "name")
	return err
}
