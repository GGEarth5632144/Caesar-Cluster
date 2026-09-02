package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"os"
	"backend/internal/dto"
	"backend/internal/entity"

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

// queryPrometheus เป็น Helper สำหรับยิง HTTP GET ไปที่ Prometheus API
func (s *TelemetryService) queryPrometheus(promQuery string) (*dto.PrometheusResponse, error) {

	promQueryURL := os.Getenv("PATH_PROMETHEUS_QUERY")

	// ต้องใช้ url.QueryEscape เพื่อแปลงช่องว่างและเครื่องหมายใน PromQL ให้ปลอดภัยสำหรับ URL
	apiURL := fmt.Sprintf(promQueryURL, url.QueryEscape(promQuery)) 
	
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
			// ถ้ายังไม่มีชื่อโหนดนี้ใน Map ให้สร้างใหม่
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
		// OnConflict จะทำการเช็กว่าถ้า NodeName ซ้ำกัน ให้ทำการ Update ฟิลด์ต่างๆ
		// ถ้าไม่ซ้ำ (เพิ่งรันครั้งแรก) ระบบจะทำการ Insert แถวใหม่ให้
		err := s.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"temperature", "ram_used_mb", "is_up", "procs", "updated_at"}),
		}).Create(&telemetryRecords).Error

		if err != nil {
			fmt.Printf("Error saving telemetry to DB: %v\n", err)
		}
	}
}

func (s *TelemetryService) GetAllTelemetry() ([]entity.NodeTelemetry, error) {
	var records []entity.NodeTelemetry
	// ใช้คำสั่ง Find เพื่อดึงข้อมูลทั้งหมดออกมา
	err := s.DB.Find(&records).Error
	return records, err
}