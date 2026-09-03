package store

import (
	"database/sql"
	"fmt"
)

// migrate brings an existing database up to the current schema. There are no
// version numbers on purpose: every step asks the database what it already
// has, so running it on a fresh file is a no-op and running it twice is safe.
func migrate(h *sql.DB) error {
	if err := widenFitFileKey(h); err != nil {
		return err
	}
	for _, c := range []struct{ table, column, def string }{
		{"fit_file", "file_index", "INTEGER NOT NULL DEFAULT 0"},
		{"sleep_session", "deep_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"sleep_session", "light_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"sleep_session", "rem_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"sleep_session", "unmeasurable_ms", "INTEGER NOT NULL DEFAULT 0"},
	} {
		has, err := hasColumn(h, c.table, c.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := h.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.column + ` ` + c.def); err != nil {
			return fmt.Errorf("store: add %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// widenFitFileKey rebuilds fit_file when it still carries the original
// UNIQUE (device_id, file_number). File numbers repeat across file types, so
// that key made a sleep file collide with a monitor file of the same number:
// the sync then skipped the download and archived the file unread on the
// watch, losing the night for good.
func widenFitFileKey(h *sql.DB) error {
	old, err := hasUniqueIndex(h, "fit_file", []string{"device_id", "file_number"})
	if err != nil || !old {
		return err
	}
	tx, err := h.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`ALTER TABLE fit_file RENAME TO fit_file_old`,
		fitFileDDL("fit_file"),
		`INSERT INTO fit_file (id, device_id, file_number, file_index, data_type, sub_type,
    file_ts, flags, size, downloaded_ms, imported, data)
SELECT id, device_id, file_number, 0, data_type, sub_type,
    file_ts, flags, size, downloaded_ms, imported, data FROM fit_file_old`,
		`DROP TABLE fit_file_old`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("store: rebuild fit_file: %w", err)
		}
	}
	return tx.Commit()
}

func hasColumn(h *sql.DB, table, column string) (bool, error) {
	rows, err := h.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("store: table info %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// hasUniqueIndex reports whether the table has a unique index covering exactly
// the given columns in order.
func hasUniqueIndex(h *sql.DB, table string, columns []string) (bool, error) {
	rows, err := h.Query(`SELECT name FROM pragma_index_list(?) WHERE "unique" = 1`, table)
	if err != nil {
		return false, fmt.Errorf("store: index list %s: %w", table, err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return false, err
		}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, name := range names {
		cols, err := indexColumns(h, name)
		if err != nil {
			return false, err
		}
		if len(cols) != len(columns) {
			continue
		}
		same := true
		for i := range cols {
			if cols[i] != columns[i] {
				same = false
				break
			}
		}
		if same {
			return true, nil
		}
	}
	return false, nil
}

func indexColumns(h *sql.DB, index string) ([]string, error) {
	rows, err := h.Query(`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, index)
	if err != nil {
		return nil, fmt.Errorf("store: index info %s: %w", index, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name sql.NullString
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name.String)
	}
	return out, rows.Err()
}
