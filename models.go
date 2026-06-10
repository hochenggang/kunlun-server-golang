package main

import "database/sql"

type FieldType string

const (
	FieldTypeCounter FieldType = "counter"
	FieldTypeGauge   FieldType = "gauge"
)

type FieldMeta struct {
	Name      string
	Type      FieldType
	GoType    string // "int" or "float64"
}

var allFields = []FieldMeta{
	{Name: "client_id", Type: FieldTypeGauge, GoType: "int"},
	{Name: "timestamp", Type: FieldTypeGauge, GoType: "int"},
	{Name: "uptime_s", Type: FieldTypeGauge, GoType: "int"},
	{Name: "load_1min", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "load_5min", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "load_15min", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "running_tasks", Type: FieldTypeGauge, GoType: "int"},
	{Name: "total_tasks", Type: FieldTypeGauge, GoType: "int"},
	{Name: "cpu_user", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_system", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_nice", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_idle", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_iowait", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_irq", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_softirq", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "cpu_steal", Type: FieldTypeCounter, GoType: "float64"},
	{Name: "mem_total_mib", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "mem_free_mib", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "mem_used_mib", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "mem_buff_cache_mib", Type: FieldTypeGauge, GoType: "float64"},
	{Name: "tcp_connections", Type: FieldTypeGauge, GoType: "int"},
	{Name: "udp_connections", Type: FieldTypeGauge, GoType: "int"},
	{Name: "default_interface_net_rx_bytes", Type: FieldTypeCounter, GoType: "int"},
	{Name: "default_interface_net_tx_bytes", Type: FieldTypeCounter, GoType: "int"},
	{Name: "cpu_num_cores", Type: FieldTypeGauge, GoType: "int"},
	{Name: "root_disk_total_kb", Type: FieldTypeGauge, GoType: "int"},
	{Name: "root_disk_avail_kb", Type: FieldTypeGauge, GoType: "int"},
	{Name: "reads_completed", Type: FieldTypeCounter, GoType: "int"},
	{Name: "writes_completed", Type: FieldTypeCounter, GoType: "int"},
	{Name: "reading_ms", Type: FieldTypeCounter, GoType: "int"},
	{Name: "writing_ms", Type: FieldTypeCounter, GoType: "int"},
	{Name: "iotime_ms", Type: FieldTypeCounter, GoType: "int"},
	{Name: "ios_in_progress", Type: FieldTypeGauge, GoType: "int"},
	{Name: "weighted_io_time", Type: FieldTypeCounter, GoType: "int"},
	{Name: "machine_id", Type: FieldTypeGauge, GoType: "string"},
	{Name: "hostname", Type: FieldTypeGauge, GoType: "string"},
}

var (
	statusFields  []string
	counterFields []string
	gaugeFields   []string
)

func init() {
	excluded := map[string]bool{"client_id": true, "machine_id": true, "hostname": true}
	for _, f := range allFields {
		if !excluded[f.Name] {
			statusFields = append(statusFields, f.Name)
		}
		if f.Type == FieldTypeCounter {
			counterFields = append(counterFields, f.Name)
		}
		if f.Type == FieldTypeGauge && !excluded[f.Name] {
			gaugeFields = append(gaugeFields, f.Name)
		}
	}
}

type KunlunReportLine struct {
	ClientID                     int            `json:"client_id"`
	Timestamp                    int            `json:"timestamp"`
	UptimeS                      int            `json:"uptime_s"`
	Load1min                     float64        `json:"load_1min"`
	Load5min                     float64        `json:"load_5min"`
	Load15min                    float64        `json:"load_15min"`
	RunningTasks                 int            `json:"running_tasks"`
	TotalTasks                   int            `json:"total_tasks"`
	CPUUser                      float64        `json:"cpu_user"`
	CPUSystem                    float64        `json:"cpu_system"`
	CPUNice                      float64        `json:"cpu_nice"`
	CPUIdle                      float64        `json:"cpu_idle"`
	CPUIOwait                    float64        `json:"cpu_iowait"`
	CPUIRQ                       float64        `json:"cpu_irq"`
	CPUSoftirq                   float64        `json:"cpu_softirq"`
	CPUSteal                     float64        `json:"cpu_steal"`
	MemTotalMiB                  float64        `json:"mem_total_mib"`
	MemFreeMiB                   float64        `json:"mem_free_mib"`
	MemUsedMiB                   float64        `json:"mem_used_mib"`
	MemBuffCacheMiB              float64        `json:"mem_buff_cache_mib"`
	TCPConnections               int            `json:"tcp_connections"`
	UDPConnections               int            `json:"udp_connections"`
	DefaultInterfaceNetRxBytes   int            `json:"default_interface_net_rx_bytes"`
	DefaultInterfaceNetTxBytes   int            `json:"default_interface_net_tx_bytes"`
	CPUNumCores                  int            `json:"cpu_num_cores"`
	RootDiskTotalKB              int            `json:"root_disk_total_kb"`
	RootDiskAvailKB              int            `json:"root_disk_avail_kb"`
	ReadsCompleted               int            `json:"reads_completed"`
	WritesCompleted              int            `json:"writes_completed"`
	ReadingMs                    int            `json:"reading_ms"`
	WritingMs                    int            `json:"writing_ms"`
	IOTimeMs                     int            `json:"iotime_ms"`
	IOSInProgress                int            `json:"ios_in_progress"`
	WeightedIOTime               int            `json:"weighted_io_time"`
	MachineID                    sql.NullString `json:"machine_id"`
	Hostname                     sql.NullString `json:"hostname"`
}

// ScanRow scans a sql.Row/sql.Rows into the struct by field order.
// Expected order: client_id, timestamp, uptime_s, load_1min, ... (allFields order)
func (k *KunlunReportLine) ScanRow(rows *sql.Rows) error {
	return rows.Scan(
		&k.ClientID,
		&k.Timestamp,
		&k.UptimeS,
		&k.Load1min,
		&k.Load5min,
		&k.Load15min,
		&k.RunningTasks,
		&k.TotalTasks,
		&k.CPUUser,
		&k.CPUSystem,
		&k.CPUNice,
		&k.CPUIdle,
		&k.CPUIOwait,
		&k.CPUIRQ,
		&k.CPUSoftirq,
		&k.CPUSteal,
		&k.MemTotalMiB,
		&k.MemFreeMiB,
		&k.MemUsedMiB,
		&k.MemBuffCacheMiB,
		&k.TCPConnections,
		&k.UDPConnections,
		&k.DefaultInterfaceNetRxBytes,
		&k.DefaultInterfaceNetTxBytes,
		&k.CPUNumCores,
		&k.RootDiskTotalKB,
		&k.RootDiskAvailKB,
		&k.ReadsCompleted,
		&k.WritesCompleted,
		&k.ReadingMs,
		&k.WritingMs,
		&k.IOTimeMs,
		&k.IOSInProgress,
		&k.WeightedIOTime,
		&k.MachineID,
		&k.Hostname,
	)
}

// Values returns the values for allFields in order, suitable for inserts.
func (k *KunlunReportLine) Values() []interface{} {
	return []interface{}{
		k.ClientID,
		k.Timestamp,
		k.UptimeS,
		k.Load1min,
		k.Load5min,
		k.Load15min,
		k.RunningTasks,
		k.TotalTasks,
		k.CPUUser,
		k.CPUSystem,
		k.CPUNice,
		k.CPUIdle,
		k.CPUIOwait,
		k.CPUIRQ,
		k.CPUSoftirq,
		k.CPUSteal,
		k.MemTotalMiB,
		k.MemFreeMiB,
		k.MemUsedMiB,
		k.MemBuffCacheMiB,
		k.TCPConnections,
		k.UDPConnections,
		k.DefaultInterfaceNetRxBytes,
		k.DefaultInterfaceNetTxBytes,
		k.CPUNumCores,
		k.RootDiskTotalKB,
		k.RootDiskAvailKB,
		k.ReadsCompleted,
		k.WritesCompleted,
		k.ReadingMs,
		k.WritingMs,
		k.IOTimeMs,
		k.IOSInProgress,
		k.WeightedIOTime,
		k.MachineID,
		k.Hostname,
	}
}

// StatusValues returns values for statusFields only (excludes client_id, machine_id, hostname).
func (k *KunlunReportLine) StatusValues() []interface{} {
	return []interface{}{
		k.Timestamp,
		k.UptimeS,
		k.Load1min,
		k.Load5min,
		k.Load15min,
		k.RunningTasks,
		k.TotalTasks,
		k.CPUUser,
		k.CPUSystem,
		k.CPUNice,
		k.CPUIdle,
		k.CPUIOwait,
		k.CPUIRQ,
		k.CPUSoftirq,
		k.CPUSteal,
		k.MemTotalMiB,
		k.MemFreeMiB,
		k.MemUsedMiB,
		k.MemBuffCacheMiB,
		k.TCPConnections,
		k.UDPConnections,
		k.DefaultInterfaceNetRxBytes,
		k.DefaultInterfaceNetTxBytes,
		k.CPUNumCores,
		k.RootDiskTotalKB,
		k.RootDiskAvailKB,
		k.ReadsCompleted,
		k.WritesCompleted,
		k.ReadingMs,
		k.WritingMs,
		k.IOTimeMs,
		k.IOSInProgress,
		k.WeightedIOTime,
	}
}

// Delta calculates the delta between new and old, applying counter logic.
func (newData *KunlunReportLine) Delta(oldData *KunlunReportLine) *KunlunReportLine {
	d := *newData // shallow copy
	for _, f := range allFields {
		if f.Type != FieldTypeCounter {
			continue
		}
		var newVal, oldVal float64
		switch f.Name {
		case "cpu_user":
			newVal, oldVal = newData.CPUUser, oldData.CPUUser
		case "cpu_system":
			newVal, oldVal = newData.CPUSystem, oldData.CPUSystem
		case "cpu_nice":
			newVal, oldVal = newData.CPUNice, oldData.CPUNice
		case "cpu_idle":
			newVal, oldVal = newData.CPUIdle, oldData.CPUIdle
		case "cpu_iowait":
			newVal, oldVal = newData.CPUIOwait, oldData.CPUIOwait
		case "cpu_irq":
			newVal, oldVal = newData.CPUIRQ, oldData.CPUIRQ
		case "cpu_softirq":
			newVal, oldVal = newData.CPUSoftirq, oldData.CPUSoftirq
		case "cpu_steal":
			newVal, oldVal = newData.CPUSteal, oldData.CPUSteal
		case "default_interface_net_rx_bytes":
			newVal = float64(newData.DefaultInterfaceNetRxBytes)
			oldVal = float64(oldData.DefaultInterfaceNetRxBytes)
		case "default_interface_net_tx_bytes":
			newVal = float64(newData.DefaultInterfaceNetTxBytes)
			oldVal = float64(oldData.DefaultInterfaceNetTxBytes)
		case "reads_completed":
			newVal = float64(newData.ReadsCompleted)
			oldVal = float64(oldData.ReadsCompleted)
		case "writes_completed":
			newVal = float64(newData.WritesCompleted)
			oldVal = float64(oldData.WritesCompleted)
		case "reading_ms":
			newVal = float64(newData.ReadingMs)
			oldVal = float64(oldData.ReadingMs)
		case "writing_ms":
			newVal = float64(newData.WritingMs)
			oldVal = float64(oldData.WritingMs)
		case "iotime_ms":
			newVal = float64(newData.IOTimeMs)
			oldVal = float64(oldData.IOTimeMs)
		case "weighted_io_time":
			newVal = float64(newData.WeightedIOTime)
			oldVal = float64(oldData.WeightedIOTime)
		}
		delta := newVal - oldVal
		switch f.Name {
		case "cpu_user":
			d.CPUUser = delta
		case "cpu_system":
			d.CPUSystem = delta
		case "cpu_nice":
			d.CPUNice = delta
		case "cpu_idle":
			d.CPUIdle = delta
		case "cpu_iowait":
			d.CPUIOwait = delta
		case "cpu_irq":
			d.CPUIRQ = delta
		case "cpu_softirq":
			d.CPUSoftirq = delta
		case "cpu_steal":
			d.CPUSteal = delta
		case "default_interface_net_rx_bytes":
			d.DefaultInterfaceNetRxBytes = int(delta)
		case "default_interface_net_tx_bytes":
			d.DefaultInterfaceNetTxBytes = int(delta)
		case "reads_completed":
			d.ReadsCompleted = int(delta)
		case "writes_completed":
			d.WritesCompleted = int(delta)
		case "reading_ms":
			d.ReadingMs = int(delta)
		case "writing_ms":
			d.WritingMs = int(delta)
		case "iotime_ms":
			d.IOTimeMs = int(delta)
		case "weighted_io_time":
			d.WeightedIOTime = int(delta)
		}
	}
	return &d
}

// RowsToTable converts sql.Rows into a table format [headers, row1, row2...].
func RowsToTable(rows *sql.Rows) ([]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := []interface{}{cols}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		scanVals := make([]interface{}, len(cols))
		for i := range vals {
			scanVals[i] = &vals[i]
		}
		if err := rows.Scan(scanVals...); err != nil {
			return nil, err
		}
		result = append(result, vals)
	}
	return result, rows.Err()
}

type AdminClientUpdate struct {
	MachineID *string `json:"machine_id"`
	Hostname  *string `json:"hostname"`
	Status    *int    `json:"status"`
}
