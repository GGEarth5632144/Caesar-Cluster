package services

import (
	"backend/internal/dto"
	"backend/internal/entity"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TelemetryService struct {
	DB *gorm.DB
}

// NewTelemetryService สร้าง instance ของ service พร้อมรับ Connection ของ Database
func NewTelemetryService(db *gorm.DB) *TelemetryService {
	return &TelemetryService{DB: db}
}

// StartTelemetryWorker ฟังก์ชันนี้ควรถูกเรียกใช้งาน 1 ครั้งตอน Start Server (เช่น ใน main.go)
func (s *TelemetryService) StartTelemetryWorker() {
	ticker := time.NewTicker(5 * time.Second)

	// สั่งรัน Goroutine แยกเป็น Background Task
	go func() {
		for range ticker.C {
			s.fetchAndSaveMetrics()
		}
	}()

	fmt.Println("Telemetry Background Worker started (5s interval)...")
}

// queryPrometheus เป็น Helper สำหรับยิง HTTP GET ไปที่ Prometheus API (แบบกันค้าง)
func (s *TelemetryService) queryPrometheus(promQuery string) (*dto.PrometheusResponse, error) {

	promQueryURL := os.Getenv("PATH_PROMETHEUS_QUERY")
	if promQueryURL == "" {
		return nil, fmt.Errorf("PATH_PROMETHEUS_QUERY is empty")
	}

	// ต้องใช้ url.QueryEscape เพื่อแปลงช่องว่างและเครื่องหมายใน PromQL ให้ปลอดภัยสำหรับ URL
	apiURL := fmt.Sprintf(promQueryURL, url.QueryEscape(promQuery))

	// 1. สร้าง Client แบบตั้งเวลา Timeout แค่ 4 วินาที
	client := &http.Client{
		Timeout: 4 * time.Second,
	}

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 2. ถ้าไม่ได้ Status 200 OK ให้ข้ามไปเลย
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Prometheus returned HTTP %d", resp.StatusCode)
	}

	var result dto.PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *TelemetryService) fetchAndSaveMetrics() {
	// 1. สร้าง Map ไว้เก็บรวบรวมข้อมูลของแต่ละโหนด (node01 - node40)
	nodesMap := make(map[string]*entity.NodeTelemetry)

	// Helper function เล็กๆ สำหรับแปลงค่า string จาก JSON เป็น float64
	parseFloat := func(val interface{}) float64 {
		strVal, ok := val.(string)
		if !ok {
			return 0
		}
		f, _ := strconv.ParseFloat(strVal, 64)
		return f
	}

	// 2. ดึงค่า Temperature
	if tempRes, err := s.queryPrometheus(`node_hwmon_temp_celsius`); err == nil && tempRes.Status == "success" {
		for _, item := range tempRes.Data.Result {
			nodeName := item.Metric["node"]
			if nodeName == "" {
				continue
			}
			if _, exists := nodesMap[nodeName]; !exists {
				nodesMap[nodeName] = &entity.NodeTelemetry{NodeName: nodeName}
			}
			nodesMap[nodeName].Temperature = parseFloat(item.Value[1])
		}
	}

	// 3. ดึงค่า RAM (แปลงจาก Bytes เป็น MB)
	if ramRes, err := s.queryPrometheus(`node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`); err == nil && ramRes.Status == "success" {
		for _, item := range ramRes.Data.Result {
			nodeName := item.Metric["node"]
			if node, exists := nodesMap[nodeName]; exists {
				node.RamUsedMB = parseFloat(item.Value[1]) / (1024 * 1024)
			}
		}
	}
	if totalRamRes, err := s.queryPrometheus(`node_memory_MemTotal_bytes`); err == nil && totalRamRes.Status == "success" {
		for _, item := range totalRamRes.Data.Result {
			nodeName := item.Metric["node"]
			if node, exists := nodesMap[nodeName]; exists {
				node.RamTotalMB = parseFloat(item.Value[1]) / (1024 * 1024)
			}
		}
	}

	var totalClusterFreeStorage float64 = 0
	if storageRes, err := s.queryPrometheus(`sum(node_filesystem_free_bytes{mountpoint="/", fstype!=""})`); err == nil && storageRes.Status == "success" {
		if len(storageRes.Data.Result) > 0 {
			valStr, _ := storageRes.Data.Result[0].Value[1].(string)
			bytesVal, _ := strconv.ParseFloat(valStr, 64)
			totalClusterFreeStorage = bytesVal / (1024 * 1024 * 1024) // แปลง Bytes เป็น GB
		}
	}
	for _, node := range nodesMap {
		node.UseableStorage = totalClusterFreeStorage
	}

	cpuQuery := `100 - (avg by (node) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`
	if cpuRes, err := s.queryPrometheus(cpuQuery); err == nil && cpuRes.Status == "success" {
		for _, item := range cpuRes.Data.Result {
			nodeName := item.Metric["node"]
			if node, exists := nodesMap[nodeName]; exists {
				node.CpuUsage = parseFloat(item.Value[1])
			}
		}
	}

	

	// 4. ดึงสถานะ IsUp
	if upRes, err := s.queryPrometheus(`up`); err == nil && upRes.Status == "success" {
		for _, item := range upRes.Data.Result {
			nodeName := item.Metric["node"]
			if node, exists := nodesMap[nodeName]; exists {
				node.IsUp = int(parseFloat(item.Value[1]))
			}
		}
	}

	// 5. ดึงจำนวน Process (ชั่วคราวแทน Pod)
	if procsRes, err := s.queryPrometheus(`node_procs_running`); err == nil && procsRes.Status == "success" {
		for _, item := range procsRes.Data.Result {
			nodeName := item.Metric["node"]
			if node, exists := nodesMap[nodeName]; exists {
				node.Procs = int(parseFloat(item.Value[1]))
			}
		}
	}

	

	// 6. แปลง Map เป็น Array (Slice) เพื่อเตรียมบันทึกลง Database
	var telemetryRecords []entity.NodeTelemetry
	for _, v := range nodesMap {
		telemetryRecords = append(telemetryRecords, *v)
	}

	// 7. บันทึกลง Database ด้วยเทคนิค Upsert (Bulk)
	if len(telemetryRecords) > 0 {
		err := s.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_name"}},
			// 🚨 [อัปเดต] เพิ่ม "ram_total_mb" และ "cpu_usage" ลงไปให้อัปเดตด้วย
			DoUpdates: clause.AssignmentColumns([]string{
				"temperature", 
				"ram_used_mb", 
				"ram_total_mb", 
				"cpu_usage", 
				"useable_storage",
				"is_up", 
				"procs", 
				"updated_at",
			}),
		}).Create(&telemetryRecords).Error

		if err != nil {
			fmt.Printf("Error saving telemetry to DB: %v\n", err)
		}
	}
}

func (s *TelemetryService) GetAllTelemetry() ([]entity.NodeTelemetry, error) {
	var records []entity.NodeTelemetry
	err := s.DB.Find(&records).Error
	return records, err
}

// --- ส่วนที่ต้องเพิ่มสำหรับ History (กราฟ) ---

// ClusterHistoryData เป็นโครงสร้างสำหรับส่งให้ React (Recharts)
type ClusterHistoryData struct {
	Time        string  `json:"time"`        // เวลาแกน X (เช่น "10:30")
	AvgTemp     float64 `json:"avgTemp"`     // อุณหภูมิเฉลี่ยคลัสเตอร์
	TotalRam    float64 `json:"totalRam"`    // แรมรวมที่ "ใช้งานอยู่" (MB)
	MaxRam      float64 `json:"maxRam"`      // 🚨 แรมรวม "ทั้งหมดที่มี" (MB) - เพิ่มมาใหม่สำหรับทำกราฟ Used/Total
	AvgCpu      float64 `json:"avgCpu"`      // 🚨 CPU เฉลี่ยทั้งคลัสเตอร์ (%) - เพิ่มมาใหม่สำหรับกราฟ
	UseableStorage     float64 `json:"useablestorage"`     
	OnlineNodes int     `json:"onlineNodes"` // จำนวนโหนดที่ออนไลน์
}

// queryPrometheusRange เป็น Helper สำหรับยิง HTTP GET ไปที่ API query_range ของ Prometheus
func (s *TelemetryService) queryPrometheusRange(promQuery string, start, end int64, step string) (*dto.PrometheusResponse, error) {
	promRangeURL := os.Getenv("PATH_PROMETHEUS_QUERY_RANGE")
	if promRangeURL == "" {
		return nil, fmt.Errorf("PATH_PROMETHEUS_QUERY_RANGE is empty")
	}

	apiURL := fmt.Sprintf(promRangeURL, url.QueryEscape(promQuery), start, end, step)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Prometheus returned HTTP %d", resp.StatusCode)
	}

	var result dto.PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetHistoryFromPrometheus ดึงข้อมูลกราฟย้อนหลังและจับคู่ข้อมูล (Merge) ตามแกนเวลา
func (s *TelemetryService) GetHistoryFromPrometheus(timeRange string) ([]ClusterHistoryData, error) {
	now := time.Now()
	end := now.Unix()
	var start int64
	var step string

	switch timeRange {
	case "1h":
		start = now.Add(-1 * time.Hour).Unix()
		step = "1m"
	case "6h":
		start = now.Add(-6 * time.Hour).Unix()
		step = "5m"
	case "24h":
		start = now.Add(-24 * time.Hour).Unix()
		step = "15m"
	case "7d":
		start = now.Add(-7 * 24 * time.Hour).Unix()
		step = "1h"
	case "30d":
		start = now.Add(-30 * 24 * time.Hour).Unix()
		step = "6h"
	default:
		start = now.Add(-1 * time.Hour).Unix()
		step = "1m"
	}

	historyMap := make(map[int64]*ClusterHistoryData)

	parseFloat := func(val interface{}) float64 {
		strVal, ok := val.(string)
		if !ok {
			return 0
		}
		f, _ := strconv.ParseFloat(strVal, 64)
		return f
	}

	// Helper สำหรับตรวจสอบและสร้าง Entry ใน Map
	ensureHistoryData := func(ts int64) {
		if _, exists := historyMap[ts]; !exists {
			// ใช้ RFC3339 ตามที่แก้บัคกราฟหน้า UI
			historyMap[ts] = &ClusterHistoryData{Time: time.Unix(ts, 0).Format(time.RFC3339)} 
		}
	}

	// 1. ดึง Avg Temp
	tempRes, _ := s.queryPrometheusRange(`avg(node_hwmon_temp_celsius)`, start, end, step)
	if tempRes != nil && tempRes.Status == "success" && len(tempRes.Data.Result) > 0 {
		for _, val := range tempRes.Data.Result[0].Values {
			ts := int64(val[0].(float64))
			ensureHistoryData(ts)
			historyMap[ts].AvgTemp = parseFloat(val[1])
		}
	}

	// 2. ดึง Total RAM Used
	ramRes, _ := s.queryPrometheusRange(`sum(node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes)`, start, end, step)
	if ramRes != nil && ramRes.Status == "success" && len(ramRes.Data.Result) > 0 {
		for _, val := range ramRes.Data.Result[0].Values {
			ts := int64(val[0].(float64))
			ensureHistoryData(ts)
			historyMap[ts].TotalRam = parseFloat(val[1]) / (1024 * 1024)
		}
	}

	// =========================================================================
	// 🚨 [เพิ่มใหม่] 2.1 ดึง Max RAM ของทั้งคลัสเตอร์ (สำหรับเอาไปโชว์ Used/Total ในกราฟ)
	// =========================================================================
	maxRamRes, _ := s.queryPrometheusRange(`sum(node_memory_MemTotal_bytes)`, start, end, step)
	if maxRamRes != nil && maxRamRes.Status == "success" && len(maxRamRes.Data.Result) > 0 {
		for _, val := range maxRamRes.Data.Result[0].Values {
			ts := int64(val[0].(float64))
			ensureHistoryData(ts)
			historyMap[ts].MaxRam = parseFloat(val[1]) / (1024 * 1024)
		}
	}

	// =========================================================================
	// 🚨 [เพิ่มใหม่] 2.2 ดึง Avg CPU ทั้งคลัสเตอร์ แบบย้อนหลัง
	// =========================================================================
	cpuQuery := `avg(100 - (avg by (node) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100))`
	cpuRes, _ := s.queryPrometheusRange(cpuQuery, start, end, step)
	if cpuRes != nil && cpuRes.Status == "success" && len(cpuRes.Data.Result) > 0 {
		for _, val := range cpuRes.Data.Result[0].Values {
			ts := int64(val[0].(float64))
			ensureHistoryData(ts)
			historyMap[ts].AvgCpu = parseFloat(val[1])
		}
	}

	storageHistoryRes, _ := s.queryPrometheusRange(`sum(node_filesystem_free_bytes{mountpoint="/", fstype!=""})`, start, end, step)
    if storageHistoryRes != nil && storageHistoryRes.Status == "success" && len(storageHistoryRes.Data.Result) > 0 {
        for _, val := range storageHistoryRes.Data.Result[0].Values {
            ts := int64(val[0].(float64))
            ensureHistoryData(ts)
            historyMap[ts].UseableStorage = parseFloat(val[1]) / (1024 * 1024 * 1024) // แปลงเป็น GB
        }
    }

	// 3. ดึง Online Nodes รวม
	upRes, _ := s.queryPrometheusRange(`sum(up)`, start, end, step)
	if upRes != nil && upRes.Status == "success" && len(upRes.Data.Result) > 0 {
		for _, val := range upRes.Data.Result[0].Values {
			ts := int64(val[0].(float64))
			ensureHistoryData(ts)
			historyMap[ts].OnlineNodes = int(parseFloat(val[1]))
		}
	}

	// เตรียมแปลงข้อมูล Map เป็น Slice และเรียงลำดับเวลา (Sort) จากเก่าไปใหม่
	var results []ClusterHistoryData
	var keys []int64
	for k := range historyMap {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, k := range keys {
		results = append(results, *historyMap[k])
	}

	return results, nil
}