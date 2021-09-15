package helpers

import (
	"context"
	"fmt"
	"strings"

	"code.cloudfoundry.org/lager"
)

// SELECT <columns> FROM <table> WHERE ... LIMIT 1 [FOR UPDATE]
func (h *sqlHelper) One(
	ctx context.Context,
	logger lager.Logger,
	q Queryable,
	table string,
	columns ColumnList,
	lockRow RowLock,
	wheres string,
	whereBindings ...interface{},
) RowScanner {
	query := fmt.Sprintf("SELECT %s FROM %s\n", strings.Join(columns, ", "), table)

	if len(wheres) > 0 {
		query += "WHERE " + wheres
	}

	query += "\nLIMIT 1"

	if lockRow {
		query += "\nFOR UPDATE"
	}

	return q.QueryRowContext(ctx, h.Rebind(query), whereBindings...)
}
