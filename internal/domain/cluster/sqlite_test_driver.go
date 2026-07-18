package cluster

import (
	"context"
	"database/sql/driver"
)

// sqliteFKDriver wraps modernc.org/sqlite to enable foreign keys by default.
// Ent expects _fk=1 support which the CGo mattn/go-sqlite3 driver handles
// natively but modernc does not.
type sqliteFKDriver struct {
	inner driver.Driver
}

func (d *sqliteFKDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}

	// Enable foreign keys on this connection
	if execCtx, ok := conn.(driver.ExecerContext); ok {
		_, _ = execCtx.ExecContext(context.Background(), "PRAGMA foreign_keys = ON", nil)
	}

	return conn, nil
}
