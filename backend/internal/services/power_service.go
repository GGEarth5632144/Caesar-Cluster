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
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			s.fetchAndSavePower()
		}
	}()
	fmt.Println("Power Background Worker started (5s interval)...")
}

func (s *PowerService) fetchAndSavePower() {
	promQueryURL := os.Getenv("GET_POWERS_URL")
	resp, err := http.Get(promQueryURL)
	if err != nil {
		fmt.Println("Error fetching power data:", err)
		return
	}
	defer resp.Body.Close()
	var kwsData dto.KWSResponse
	if err := json.NewDecoder(resp.Body).Decode(&kwsData); err != nil {
		fmt.Println("Error decoding power data:", err)
		return
	}

	

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
		}

		var totalWatt float64
        var totalAmp float64
        var totalVolt float64
        var activeCount int

        for _, n := range powerRecords {
            totalWatt += n.Watt
            totalAmp += n.Amp
            // เอาเฉพาะตัวที่น่าจะทำงานจริงมาคิดค่าเฉลี่ย Volt (หลีกเลี่ยงตัวที่ดึงไฟไม่ถึง)
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

        // บันทึกลงตารางประวัติ (PowerHistory)
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