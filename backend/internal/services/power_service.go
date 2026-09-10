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

// powerHistoryWindow แปลง timeRange ที่ React ส่งมาเป็น "ย้อนหลังเท่าไหร่" + "หนึ่งจุดกราฟกว้างกี่วินาที"
//
// ความกว้างของ bucket ตั้งให้ตรงกับ step ของ TelemetryService.GetHistoryFromPrometheus เป๊ะๆ
// กราฟสองอันบนหน้า Dashboard เดียวกันจะได้มีความละเอียดตามแกนเวลาเท่ากัน อ่านเทียบกันได้จริง
//
// ทุกช่วงจบที่ 60-170 จุด ซึ่งเกินความละเอียดที่จอกว้าง ~1400px จะวาดแยกออกอยู่แล้ว
func powerHistoryWindow(timeRange string) (lookback time.Duration, bucketSeconds int) {
	switch timeRange {
	case "6h":
		return 6 * time.Hour, 5 * 60 // 72 จุด
	case "24h":
		return 24 * time.Hour, 15 * 60 // 96 จุด
	case "7d":
		return 7 * 24 * time.Hour, 60 * 60 // 168 จุด
	case "30d":
		return 30 * 24 * time.Hour, 6 * 60 * 60 // 120 จุด
	default: // "1h" และค่าที่ไม่รู้จัก
		return time.Hour, 60 // 60 จุด
	}
}

// GetPowerHistory คืนกราฟการใช้ไฟย้อนหลัง โดย "ยุบข้อมูลให้เหลือเท่าที่กราฟวาดได้จริง" ตั้งแต่ใน SQL
//
// worker เขียน power_histories ทุก 15 วินาที = 5,760 แถวต่อวัน ถ้าส่งแถวดิบทั้งหมดออกไป
// ช่วง 24h จะเป็น ~5,700 จุด และ 30d จะเป็น ~172,000 จุด (หลาย MB) ทั้งที่ recharts
// วาดได้จริงแค่ระดับร้อยจุด — ที่เหลือคือ byte ที่วิ่งข้ามเน็ตไปให้ browser ทิ้งเปล่าๆ
// แล้วหน้า AdminDashboard ยังดึงซ้ำทุก 30 วินาทีอีก
//
// data flow: timeRange → หา lookback + ความกว้าง bucket → ให้ Postgres GROUP BY ตามช่วงเวลา
// แล้ว AVG ค่าในแต่ละช่วง → ได้ผลลัพธ์ระดับร้อยแถวส่งกลับตรงๆ (ไม่ต้องวนรวมเองใน Go อีก)
func (s *PowerService) GetPowerHistory(timeRange string) ([]dto.PowerHistoryResponse, error) {
	lookback, bucketSeconds := powerHistoryWindow(timeRange)
	startTime := time.Now().Add(-lookback)

	// "timestamp" ต้องใส่ quote เพราะเป็นชื่อชนิดข้อมูลของ Postgres ด้วย — ถ้าไม่ quote
	// ตัว parser จะอ่าน extract(epoch from timestamp) เป็นการอ้างถึง type ไม่ใช่คอลัมน์
	//
	// floor(epoch / bucket) * bucket = ปัดเวลาลงให้ตกขอบ bucket เดียวกัน แล้วค่อย GROUP BY
	// (ไม่ใช้ date_trunc เพราะมันปัดได้แค่หน่วยสำเร็จรูป hour/day ไม่รองรับ 5 หรือ 15 นาที)
	// เขียนเป็น Raw SQL ตรงๆ ไม่ผ่าน query builder เพราะ GORM ใส่ quote ให้อาร์กิวเมนต์ของ
	// Group() เสมอ — "GROUP BY 1" จะกลายเป็น GROUP BY "1" ซึ่ง Postgres อ่านเป็น "คอลัมน์
	// ชื่อ 1" แล้วตอบว่าไม่มีคอลัมน์นี้ ส่วน Raw ส่ง SQL ไปตามที่เขียนไว้ทุกตัวอักษร
	var responses []dto.PowerHistoryResponse
	err := s.DB.Raw(`
		SELECT to_timestamp(floor(extract(epoch from "timestamp") / ?) * ?) AS time,
		       AVG(total_watt) AS total_watt,
		       AVG(total_amp)  AS total_amp,
		       AVG(avg_volt)   AS avg_volt
		FROM power_histories
		WHERE "timestamp" >= ?
		GROUP BY 1
		ORDER BY 1`, bucketSeconds, bucketSeconds, startTime).Scan(&responses).Error
	if err != nil {
		return nil, err
	}

	// ถ้าไม่มีข้อมูล ให้คืน slice ว่างๆ ป้องกัน null exception ในฝั่ง React
	if responses == nil {
		responses = []dto.PowerHistoryResponse{}
	}

	return responses, nil
}