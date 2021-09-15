package db

import (
	"context"

	"code.cloudfoundry.org/lager"
)

func (db *SQLDB) CreateLockTable(ctx context.Context, logger lager.Logger) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS locks (
			path VARCHAR(255) PRIMARY KEY,
			owner VARCHAR(255),
			value VARCHAR(4096),
			type VARCHAR(255) DEFAULT '',
			modified_index BIGINT DEFAULT 0,
			modified_id varchar(255) DEFAULT '',
			ttl BIGINT DEFAULT 0
		);
	`)
	if err != nil {
		return err
	}

	return nil
}
