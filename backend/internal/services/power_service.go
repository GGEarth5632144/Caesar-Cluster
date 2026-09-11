package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"os"
	"backend/internal/dto"
	"backend/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PowerService struct {
	DB *gorm.DB
}

func NewPowerService(db *gorm.DB) *PowerService {
	return &PowerService{DB: db}
}

func (s *PowerService) StartPowerWorker() {
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		for range ticker.C {
			s.fetchAndSavePower()
		}
	}()
	fmt.Println("Power Background Worker started (5s interval)...")
}

func (s *PowerService) fetchAndSavePower() {
	promQueryURL := os.Getenv("GET_POWERS_URL")
	if promQueryURL == "" {
		fmt.Println("Warning: GET_POWERS_URL is empty")
		return
	}

	// 1. สร้าง Request ด้วยตัวเองแทนการใช้ http.Get ตรงๆ
	req, err := http.NewRequest("GET", promQueryURL, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// 🚨 ท่าไม้ตายที่ 1: บังคับปิด Connection ทันที (ป้องกันบอร์ด ESP ซ็อกเก็ตเต็ม)
	req.Close = true 
	
	// 🚨 ท่าไม้ตายที่ 2: ปลอมตัวเป็น Browser เผื่อบอร์ดมันเตะบอท
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36")

	// 🚨 ท่าไม้ตายที่ 3: ปิด Keep-Alive ที่ฝั่ง Transport ควบคู่กันไป
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,  
			Proxy:             nil,   
			ForceAttemptHTTP2: false, 
		},
	}

	// ใช้ client.Do(req) แทน client.Get
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Skip this tick - Fetch error or Timeout:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Skip this tick - Device returned HTTP %d\n", resp.StatusCode)
		return
	}

	// Decode JSON 
	var kwsData dto.KWSResponse
	if err := json.NewDecoder(resp.Body).Decode(&kwsData); err != nil {
		fmt.Println("Error decoding power data:", err)
		return
	}

	// ... (ส่วนโค้ด For loop คำนวณ watt และบันทึกลง Database ให้คงไว้เหมือนเดิมเป๊ะๆ เลยครับ) ...
	var powerRecords []entity.PowerNode

	for _, child := range kwsData.Children {
		volt, _ := strconv.ParseFloat(strings.TrimSpace(child.Volt), 64)
		amp, _ := strconv.ParseFloat(strings.TrimSpace(child.Amp), 64)
		hz, _ := strconv.ParseFloat(strings.TrimSpace(child.Hz), 64)
		pf, _ := strconv.ParseFloat(strings.TrimSpace(child.PF), 64)
		whr, _ := strconv.ParseFloat(strings.TrimSpace(child.Whr), 64)

		watt := volt * amp * pf

		record := entity.PowerNode{
			ModbusID: child.ModbusID,
			Status:   child.Status,
			Volt:     volt,
			Amp:      amp,
			Hz:       hz,
			PF:       pf,
			Whr:      whr,
			Watt:     watt,
		}
		powerRecords = append(powerRecords, record)
	}

	if len(powerRecords) > 0 {
		err := s.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "modbus_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "volt", "amp", "hz", "pf", "whr", "watt", "updated_at"}),
		}).Create(&powerRecords).Error

		if err != nil {
			fmt.Printf("Error saving power metrics: %v\n", err)
			return
		}

		var totalWatt float64
		var totalAmp float64
		var totalVolt float64
		var activeCount int

		for _, n := range powerRecords {
			totalWatt += n.Watt
			totalAmp += n.Amp
			if n.Volt > 0 {
				totalVolt += n.Volt
				activeCount++
			}
		}

		avgVolt := float64(0)
		if activeCount > 0 {
			avgVolt = totalVolt / float64(activeCount)
		}

		historyRecord := entity.PowerHistory{
			Timestamp: time.Now(),
			TotalWatt: totalWatt,
			TotalAmp:  totalAmp,
			AvgVolt:   avgVolt,
		}

		if err := s.DB.Create(&historyRecord).Error; err != nil {
			fmt.Printf("Error saving power history: %v\n", err)
		}
	}
}

// สำหรับให้ Controller เรียกใช้เพื่อส่งข้อมูลไปที่หน้าเว็บ
func (s *PowerService) GetAllPower() ([]entity.PowerNode, error) {
	var records []entity.PowerNode
	err := s.DB.Find(&records).Error
	return records, err
}

func (s *PowerService) GetPowerHistory(timeRange string) ([]dto.PowerHistoryResponse, error) {
	var histories []entity.PowerHistory
	now := time.Now()
	var startTime time.Time

	// แปลง timeRange ที่รับมาจาก React ให้เป็นระยะเวลา
	switch timeRange {
	case "1h":
		startTime = now.Add(-1 * time.Hour)
	case "6h":
		startTime = now.Add(-6 * time.Hour)
	case "24h":
		startTime = now.Add(-24 * time.Hour)
	case "7d":
		startTime = now.Add(-7 * 24 * time.Hour)
	case "30d":
		startTime = now.Add(-30 * 24 * time.Hour)
	default:
		startTime = now.Add(-1 * time.Hour)
	}

	// ดึงจากฐานข้อมูล เรียงตามเวลาเก่าไปใหม่
	err := s.DB.Where("timestamp >= ?", startTime).
		Order("timestamp ASC").
		Find(&histories).Error

	if err != nil {
		return nil, err
	}

	// สร้าง DTO Response
	var responses []dto.PowerHistoryResponse
	for _, h := range histories {
		responses = append(responses, dto.PowerHistoryResponse{
			Time:      h.Timestamp,
			TotalWatt: h.TotalWatt,
			TotalAmp:  h.TotalAmp,
			AvgVolt:   h.AvgVolt,
		})
	}

	// ถ้าไม่มีข้อมูล ให้คืน slice ว่างๆ ป้องกัน null exception ในฝั่ง React
	if responses == nil {
		responses = []dto.PowerHistoryResponse{}
	}

	return responses, nil
}