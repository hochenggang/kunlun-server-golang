package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	db       *sql.DB
	database = "db/kunlun_status.db"
)

func initDB() error {
	os.MkdirAll("db", 0755)

	var err error
	db, err = sql.Open("sqlite", database+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=10000;"); err != nil {
		return err
	}

	columnDefs := getDBColumnDef()

	schema := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS client (
			id INTEGER PRIMARY KEY NOT NULL,
			ip TEXT,
			machine_id TEXT UNIQUE NOT NULL,
			hostname TEXT NOT NULL,
			status INTEGER NOT NULL DEFAULT 0,
			last_update INTEGER NOT NULL,
			create_ts INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS status_latest (
			client_id INTEGER PRIMARY KEY,
			%s,
			FOREIGN KEY (client_id) REFERENCES client(id)
		);

		CREATE TABLE IF NOT EXISTS status_seconds (
			client_id INTEGER NOT NULL,
			%s,
			PRIMARY KEY (client_id, timestamp),
			FOREIGN KEY (client_id) REFERENCES client(id)
		);

		CREATE TABLE IF NOT EXISTS status_minutes (
			client_id INTEGER NOT NULL,
			%s,
			PRIMARY KEY (client_id, timestamp),
			FOREIGN KEY (client_id) REFERENCES client(id)
		);

		CREATE TABLE IF NOT EXISTS status_hours (
			client_id INTEGER NOT NULL,
			%s,
			PRIMARY KEY (client_id, timestamp),
			FOREIGN KEY (client_id) REFERENCES client(id)
		);
	`, columnDefs, columnDefs, columnDefs, columnDefs)

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Migration: add missing columns to client table
	cols, err := getTableColumns("client")
	if err != nil {
		return err
	}
	colMap := make(map[string]bool)
	for _, c := range cols {
		colMap[c] = true
	}
	if !colMap["status"] {
		if _, err := db.Exec("ALTER TABLE client ADD COLUMN status INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if !colMap["last_update"] {
		if _, err := db.Exec("ALTER TABLE client ADD COLUMN last_update INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if !colMap["ip"] {
		if _, err := db.Exec("ALTER TABLE client ADD COLUMN ip TEXT"); err != nil {
			return err
		}
	}

	return nil
}

func getDBColumnDef() string {
	var parts []string
	for _, f := range allFields {
		if f.Name == "client_id" || f.Name == "machine_id" || f.Name == "hostname" {
			continue
		}
		dbType := "REAL NOT NULL"
		if f.GoType == "int" {
			dbType = "INTEGER NOT NULL"
		}
		parts = append(parts, fmt.Sprintf("%s %s", f.Name, dbType))
	}
	return strings.Join(parts, ",\n\t\t\t")
}

func getTableColumns(table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func getOrCreateClient(machineID, hostname, clientIP string) (int, int, error) {
	currentTs := int(time.Now().Unix())

	var clientID, status int
	var existingHostname string
	err := db.QueryRow("SELECT id, hostname, status FROM client WHERE machine_id = ?", machineID).Scan(&clientID, &existingHostname, &status)
	if err == sql.ErrNoRows {
		var maxID sql.NullInt64
		if err := db.QueryRow("SELECT MAX(id) AS max_id FROM client").Scan(&maxID); err != nil {
			return 0, 0, err
		}
		newID := 1
		if maxID.Valid {
			newID = int(maxID.Int64) + 1
		}
		_, err := db.Exec(
			"INSERT INTO client (id, machine_id, hostname, status, ip, last_update, create_ts) VALUES (?, ?, ?, 0, ?, ?, ?)",
			newID, machineID, hostname, clientIP, currentTs, currentTs,
		)
		if err != nil {
			return 0, 0, err
		}
		return newID, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}

	if existingHostname != hostname {
		_, err = db.Exec("UPDATE client SET hostname = ?, ip = ?, last_update = ? WHERE id = ?", hostname, clientIP, currentTs, clientID)
	} else {
		_, err = db.Exec("UPDATE client SET ip = ?, last_update = ? WHERE id = ?", clientIP, currentTs, clientID)
	}
	if err != nil {
		return 0, 0, err
	}
	return clientID, status, nil
}

func getLastStatus(clientID int) (*KunlunReportLine, error) {
	rows, err := db.Query("SELECT * FROM status_latest WHERE client_id = ? LIMIT 1", clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	k := &KunlunReportLine{}
	if err := k.ScanRow(rows); err != nil {
		return nil, err
	}
	return k, nil
}

func insertStatusLatest(clientID int, line *KunlunReportLine) error {
	fields := []string{"client_id"}
	fields = append(fields, statusFields...)
	placeholders := make([]string, len(fields))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	vals := []interface{}{clientID}
	vals = append(vals, line.StatusValues()...)

	sqlStr := fmt.Sprintf("INSERT OR REPLACE INTO status_latest (%s) VALUES (%s)", strings.Join(fields, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(sqlStr, vals...)
	return err
}

func insertStatusSeconds(clientID int, line *KunlunReportLine) error {
	fields := []string{"client_id"}
	fields = append(fields, statusFields...)
	placeholders := make([]string, len(fields))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	vals := []interface{}{clientID}
	vals = append(vals, line.StatusValues()...)

	sqlStr := fmt.Sprintf("INSERT OR REPLACE INTO status_seconds (%s) VALUES (%s)", strings.Join(fields, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(sqlStr, vals...)
	return err
}

func cleanupStatusSeconds(clientID int) error {
	_, err := db.Exec(`
		DELETE FROM status_seconds
		WHERE (client_id, timestamp) IN (
			SELECT client_id, timestamp FROM status_seconds
			WHERE client_id = ? ORDER BY timestamp DESC LIMIT -1 OFFSET 360
		)
	`, clientID)
	return err
}

func rollupStatusMinutes(clientID, timestamp int) error {
	sqlStr := generateAggregateSQL("status_seconds", "status_minutes", 60)
	_, err := db.Exec(sqlStr, clientID, timestamp)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		DELETE FROM status_minutes
		WHERE (client_id, timestamp) IN (
			SELECT client_id, timestamp FROM status_minutes
			WHERE client_id = ? ORDER BY timestamp DESC LIMIT -1 OFFSET 1440
		)
	`, clientID)
	return err
}

func rollupStatusHours(clientID, timestamp int) error {
	sqlStr := generateAggregateSQL("status_minutes", "status_hours", 3600)
	_, err := db.Exec(sqlStr, clientID, timestamp)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		DELETE FROM status_hours
		WHERE (client_id, timestamp) IN (
			SELECT client_id, timestamp FROM status_hours
			WHERE client_id = ? ORDER BY timestamp DESC LIMIT -1 OFFSET 8760
		)
	`, clientID)
	return err
}

func generateAggregateSQL(sourceTable, targetTable string, intervalSeconds int) string {
	var selectParts []string
	selectParts = append(selectParts, "client_id", "MAX(timestamp) AS timestamp")

	for _, f := range statusFields {
		if f == "client_id" || f == "timestamp" {
			continue
		}
		if isCounterField(f) {
			selectParts = append(selectParts, fmt.Sprintf("SUM(%s) AS %s", f, f))
		} else {
			selectParts = append(selectParts, fmt.Sprintf("ROUND(AVG(%s), 2) AS %s", f, f))
		}
	}

	insertFields := []string{"client_id", "timestamp"}
	for _, f := range statusFields {
		if f != "client_id" && f != "timestamp" {
			insertFields = append(insertFields, f)
		}
	}

	return fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (%s)
		SELECT %s
		FROM %s
		WHERE client_id = ? AND timestamp >= ? - %d
		GROUP BY client_id
	`, targetTable, strings.Join(insertFields, ", "), strings.Join(selectParts, ", "), sourceTable, intervalSeconds)
}

func isCounterField(name string) bool {
	for _, f := range counterFields {
		if f == name {
			return true
		}
	}
	return false
}

func getStatusLatestTable() ([]interface{}, error) {
	rows, err := db.Query(`
		SELECT sl.*, c.machine_id, c.hostname
		FROM status_latest sl
		JOIN client c ON sl.client_id = c.id
		WHERE sl.timestamp = (
			SELECT MAX(timestamp)
			FROM status_latest
			WHERE client_id = sl.client_id
		)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return RowsToTable(rows)
}

func getStatusSeconds(clientID, limit int) ([]interface{}, error) {
	rows, err := db.Query("SELECT * FROM status_seconds WHERE client_id = ? ORDER BY timestamp DESC LIMIT ?", clientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return RowsToTable(rows)
}

func getStatusMinutes(clientID, limit int) ([]interface{}, error) {
	rows, err := db.Query("SELECT * FROM status_minutes WHERE client_id = ? ORDER BY timestamp DESC LIMIT ?", clientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return RowsToTable(rows)
}

func getStatusHours(clientID, limit int) ([]interface{}, error) {
	rows, err := db.Query("SELECT * FROM status_hours WHERE client_id = ? ORDER BY timestamp DESC LIMIT ?", clientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return RowsToTable(rows)
}

func getClients() ([]map[string]interface{}, error) {
	rows, err := db.Query("SELECT * FROM client ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		scanVals := make([]interface{}, len(cols))
		for i := range vals {
			scanVals[i] = &vals[i]
		}
		if err := rows.Scan(scanVals...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{})
		for i, c := range cols {
			row[c] = vals[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func updateClient(clientID int, update *AdminClientUpdate) error {
	var sets []string
	var vals []interface{}
	if update.MachineID != nil {
		sets = append(sets, "machine_id = ?")
		vals = append(vals, *update.MachineID)
	}
	if update.Hostname != nil {
		sets = append(sets, "hostname = ?")
		vals = append(vals, *update.Hostname)
	}
	if update.Status != nil {
		sets = append(sets, "status = ?")
		vals = append(vals, *update.Status)
	}
	if len(sets) == 0 {
		return nil
	}
	vals = append(vals, clientID)
	sqlStr := fmt.Sprintf("UPDATE client SET %s WHERE id = ?", strings.Join(sets, ", "))
	_, err := db.Exec(sqlStr, vals...)
	return err
}

func deleteClient(clientID int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM status_latest WHERE client_id = ?", clientID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM status_seconds WHERE client_id = ?", clientID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM status_minutes WHERE client_id = ?", clientID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM status_hours WHERE client_id = ?", clientID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM client WHERE id = ?", clientID); err != nil {
		return err
	}
	return tx.Commit()
}
