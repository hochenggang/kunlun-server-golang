package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	adminToken = os.Getenv("ADMIN_TOKEN")
	indexHTML  []byte
	indexOnce  sync.Once
)

func init() {
	if adminToken == "" {
		adminToken = "Admin123"
	}
}

func getClientIP(c *gin.Context) string {
	xff := c.GetHeader("X-Forwarded-For")
	if xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	xri := c.GetHeader("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}
	return c.ClientIP()
}

func verifyAdminToken(c *gin.Context) bool {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return false
	}
	auth = strings.TrimSpace(auth)
	if strings.HasPrefix(auth, "Bearer ") {
		auth = auth[7:]
	}
	return auth == adminToken
}

func setupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// CORS
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Global panic recovery returning 400 plaintext
	r.Use(func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				c.String(http.StatusBadRequest, fmt.Sprintf("%v", rec))
				c.Abort()
			}
		}()
		c.Next()
	})

	r.POST("/status", handlePostStatus)
	r.GET("/status", func(c *gin.Context) {
		c.String(http.StatusOK, "kunlun")
	})
	r.GET("/status/latest", handleGetStatusLatest)
	r.GET("/status/seconds", handleGetStatusSeconds)
	r.GET("/status/minutes", handleGetStatusMinutes)
	r.GET("/status/hours", handleGetStatusHours)
	r.GET("/admin/client", handleAdminGetClients)
	r.PUT("/admin/client/:id", handleAdminUpdateClient)
	r.DELETE("/admin/client/:id", handleAdminDeleteClient)
	r.GET("/", handleIndex)
	r.NoRoute(func(c *gin.Context) {
		c.Header("Content-Type", "text/html")
		c.String(http.StatusNotFound, `<center style='
            display: flex;
            align-items: center;
            justify-content: center;
            width: 100%;
            height: 100%;
            position: fixed;'>
            404 Not Found
        </center>`)
	})

	return r
}

func handlePostStatus(c *gin.Context) {
	valuesStr := c.PostForm("values")
	valuesList := strings.Split(valuesStr, ",")
	expectedLen := len(statusFields) + 2 // machine_id, hostname
	if len(valuesList) != expectedLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("required fields %d, recived %d", expectedLen, len(valuesList)),
		})
		return
	}

	line, err := parseReportLine(valuesList)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if line.Timestamp%10 != 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("required timestamp must %% 10 = 0, recived timestamp %d %% 10 = %d", line.Timestamp, line.Timestamp%10),
		})
		return
	}

	clientIP := getClientIP(c)
	clientID, status, err := getOrCreateClient(line.MachineID.String, line.Hostname.String, clientIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	line.ClientID = clientID

	if status != 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "client not approved, status=0, waiting for admin approval"})
		return
	}

	lastLine, err := getLastStatus(clientID)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := insertStatusLatest(clientID, line); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if lastLine == nil {
		c.JSON(http.StatusOK, gin.H{"ok": 1})
		return
	}

	delta := line.Delta(lastLine)
	if err := insertStatusSeconds(clientID, delta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := cleanupStatusSeconds(clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if line.Timestamp%60 == 0 {
		if err := rollupStatusMinutes(clientID, line.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if line.Timestamp%3600 == 0 {
		if err := rollupStatusHours(clientID, line.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": 2})
}

func parseReportLine(values []string) (*KunlunReportLine, error) {
	// values order: statusFields..., machine_id, hostname
	k := &KunlunReportLine{}
	idx := 0

	parseInt := func() (int, error) {
		v, err := strconv.Atoi(strings.TrimSpace(values[idx]))
		idx++
		return v, err
	}
	parseFloat := func() (float64, error) {
		v, err := strconv.ParseFloat(strings.TrimSpace(values[idx]), 64)
		idx++
		return v, err
	}

	var err error
	k.Timestamp, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}
	k.UptimeS, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid uptime_s: %w", err)
	}
	k.Load1min, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid load_1min: %w", err)
	}
	k.Load5min, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid load_5min: %w", err)
	}
	k.Load15min, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid load_15min: %w", err)
	}
	k.RunningTasks, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid running_tasks: %w", err)
	}
	k.TotalTasks, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid total_tasks: %w", err)
	}
	k.CPUUser, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_user: %w", err)
	}
	k.CPUSystem, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_system: %w", err)
	}
	k.CPUNice, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_nice: %w", err)
	}
	k.CPUIdle, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_idle: %w", err)
	}
	k.CPUIOwait, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_iowait: %w", err)
	}
	k.CPUIRQ, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_irq: %w", err)
	}
	k.CPUSoftirq, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_softirq: %w", err)
	}
	k.CPUSteal, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_steal: %w", err)
	}
	k.MemTotalMiB, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid mem_total_mib: %w", err)
	}
	k.MemFreeMiB, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid mem_free_mib: %w", err)
	}
	k.MemUsedMiB, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid mem_used_mib: %w", err)
	}
	k.MemBuffCacheMiB, err = parseFloat()
	if err != nil {
		return nil, fmt.Errorf("invalid mem_buff_cache_mib: %w", err)
	}
	k.TCPConnections, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid tcp_connections: %w", err)
	}
	k.UDPConnections, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid udp_connections: %w", err)
	}
	k.DefaultInterfaceNetRxBytes, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid default_interface_net_rx_bytes: %w", err)
	}
	k.DefaultInterfaceNetTxBytes, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid default_interface_net_tx_bytes: %w", err)
	}
	k.CPUNumCores, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid cpu_num_cores: %w", err)
	}
	k.RootDiskTotalKB, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid root_disk_total_kb: %w", err)
	}
	k.RootDiskAvailKB, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid root_disk_avail_kb: %w", err)
	}
	k.ReadsCompleted, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid reads_completed: %w", err)
	}
	k.WritesCompleted, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid writes_completed: %w", err)
	}
	k.ReadingMs, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid reading_ms: %w", err)
	}
	k.WritingMs, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid writing_ms: %w", err)
	}
	k.IOTimeMs, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid iotime_ms: %w", err)
	}
	k.IOSInProgress, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid ios_in_progress: %w", err)
	}
	k.WeightedIOTime, err = parseInt()
	if err != nil {
		return nil, fmt.Errorf("invalid weighted_io_time: %w", err)
	}

	k.MachineID.String = strings.TrimSpace(values[idx])
	k.MachineID.Valid = k.MachineID.String != ""
	idx++
	k.Hostname.String = strings.TrimSpace(values[idx])
	k.Hostname.Valid = true
	if k.Hostname.String == "" {
		k.Hostname.String = "unknown"
	}

	return k, nil
}

func handleGetStatusLatest(c *gin.Context) {
	data, err := getStatusLatestTable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func handleGetStatusSeconds(c *gin.Context) {
	clientID, _ := strconv.Atoi(c.Query("client_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 360
	}
	data, err := getStatusSeconds(clientID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func handleGetStatusMinutes(c *gin.Context) {
	clientID, _ := strconv.Atoi(c.Query("client_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 1440
	}
	data, err := getStatusMinutes(clientID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func handleGetStatusHours(c *gin.Context) {
	clientID, _ := strconv.Atoi(c.Query("client_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 8760
	}
	data, err := getStatusHours(clientID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func handleAdminGetClients(c *gin.Context) {
	if !verifyAdminToken(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	clients, err := getClients()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, clients)
}

func handleAdminUpdateClient(c *gin.Context) {
	if !verifyAdminToken(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	var update AdminClientUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if update.MachineID == nil && update.Hostname == nil && update.Status == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "no fields to update"})
		return
	}

	if err := updateClient(clientID, &update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	clients, err := getClients()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var updated map[string]interface{}
	for _, cl := range clients {
		if int(cl["id"].(int64)) == clientID {
			updated = cl
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "client": updated})
}

func handleAdminDeleteClient(c *gin.Context) {
	if !verifyAdminToken(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}
	if err := deleteClient(clientID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": fmt.Sprintf("client %d and all related data deleted", clientID)})
}

func handleIndex(c *gin.Context) {
	indexOnce.Do(func() {
		resp, err := http.Get("https://github.com/hochenggang/kunlun-frontend/releases/latest/download/index.html")
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			indexHTML = body
		}
	})
	c.Data(http.StatusOK, "text/html", indexHTML)
}
