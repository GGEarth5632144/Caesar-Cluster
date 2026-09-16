package services

import (
	"backend/internal/dto"
	"backend/internal/entity"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PowerService struct {
	DB *gorm.DB

	// เก็บประวัติย้อนหลังกี่วัน (<= 0 = ไม่ลบเลย) ห้ามน้อยกว่าช่วงยาวสุดของกราฟ (30d)
	retentionDays int
}

func NewPowerService(db *gorm.DB, retentionDays int) *PowerService {
	return &PowerService{DB: db, retentionDays: retentionDays}
}

const (
	powerWorkerInterval = 15 * time.Second // ความถี่ที่ดึงค่าจากมิเตอร์
	powerPruneInterval  = 6 * time.Hour    // ความถี่ที่ไล่ลบของเก่า
)

func (s *PowerService) StartPowerWorker() {
	ticker := time.NewTicker(powerWorkerInterval)
	go func() {
		for range ticker.C {
			s.fetchAndSavePower()
		}
	}()
	log.Printf("power worker started (%s interval)", powerWorkerInterval)

	s.startHistoryPruner()
}

// startHistoryPruner ลบแถวเก่าของ power_histories ทิ้งเป็นระยะ
//
// power_histories เป็นตารางเดียวที่โตไม่หยุด (5,760 แถว/วัน ~2 ล้านแถว/ปี) ส่วนตารางอื่น
// upsert ทับของเดิม และไม่มีใครอ่านข้อมูลที่เก่ากว่าช่วงยาวสุดของกราฟเลย
//
// ลบรอบแรกทันทีตอน start ไม่รอครบ 6 ชม. เพื่อตัดของที่สะสมไว้ก่อนมีโค้ดนี้
func (s *PowerService) startHistoryPruner() {
	if s.retentionDays <= 0 {
		log.Println("power history pruner disabled (retention <= 0) — เก็บข้อมูลย้อนหลังทั้งหมด")
		return
	}

	go func() {
		s.prunePowerHistory()
		ticker := time.NewTicker(powerPruneInterval)
		for range ticker.C {
			s.prunePowerHistory()
		}
	}()
	log.Printf("power history pruner started (keep %d days, run every %s)", s.retentionDays, powerPruneInterval)
}

// prunePowerHistory ลบแถวที่เก่ากว่าช่วงที่ตั้งไว้ — ล้มเหลวก็แค่ log ไม่ทำให้ worker หยุด
func (s *PowerService) prunePowerHistory() {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)
	res := s.DB.Where(`"timestamp" < ?`, cutoff).Delete(&entity.PowerHistory{})
	if res.Error != nil {
		log.Printf("prune power history error: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("pruned %d power history rows older than %s", res.RowsAffected, cutoff.Format(time.RFC3339))
	}
}

func (s *PowerService) fetchAndSavePower() {
	promQueryURL := os.Getenv("GET_POWERS_URL")
	if promQueryURL == "" {
		log.Println("คำเตือน: GET_POWERS_URL ว่าง — ข้ามการเก็บค่าการใช้ไฟ")
		return
	}

	// สร้าง request เองแทน http.Get เพื่อคุมสามอย่างที่บอร์ด ESP ต้องการ: ปิด connection ทันที
	// (กันซ็อกเก็ตเต็ม), ปลอม User-Agent (บอร์ดเตะ client ที่ไม่เหมือน browser) และปิด keep-alive
	req, err := http.NewRequest("GET", promQueryURL, nil)
	if err != nil {
		log.Printf("power: สร้าง request ไม่สำเร็จ: %v", err)
		return
	}

	req.Close = true
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			Proxy:             nil,
			ForceAttemptHTTP2: false,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("power: ข้ามรอบนี้ (ดึงค่าไม่สำเร็จ/timeout): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("power: ข้ามรอบนี้ (อุปกรณ์ตอบ HTTP %d)", resp.StatusCode)
		return
	}

	var kwsData dto.KWSResponse
	if err := json.NewDecoder(resp.Body).Decode(&kwsData); err != nil {
		log.Printf("power: อ่าน JSON จากอุปกรณ์ไม่สำเร็จ: %v", err)
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
			log.Printf("power: บันทึกค่ามิเตอร์ไม่สำเร็จ: %v", err)
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
			log.Printf("power: บันทึกประวัติการใช้ไฟไม่สำเร็จ: %v", err)
		}
	}
}

// GetAllPower คืนค่าล่าสุดของมิเตอร์ทุกตัว (หนึ่งแถวต่อ modbus_id)
func (s *PowerService) GetAllPower() ([]entity.PowerNode, error) {
	var records []entity.PowerNode
	err := s.DB.Find(&records).Error
	return records, err
}

// powerHistoryWindow แปลง timeRange เป็น "ย้อนหลังเท่าไหร่" + "หนึ่งจุดกราฟกว้างกี่วินาที"
//
// ขนาด bucket ตรงกับ step ของ TelemetryService.GetHistoryFromPrometheus เป๊ะๆ กราฟสองอัน
// บนหน้าเดียวกันจะได้อ่านเทียบกันได้ ทุกช่วงจบที่ 60-170 จุด ซึ่งพอสำหรับจอกว้าง ~1400px
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

// GetPowerHistory คืนกราฟการใช้ไฟย้อนหลัง โดยยุบข้อมูลให้เหลือเท่าที่กราฟวาดได้จริงตั้งแต่ใน SQL
//
// ส่งแถวดิบทั้งหมดคือ ~5,700 จุดที่ช่วง 24h และ ~172,000 จุด (หลาย MB) ที่ 30d ทั้งที่ recharts
// วาดได้แค่ระดับร้อยจุด และหน้า AdminDashboard ยังดึงซ้ำทุก 30 วินาทีอีก
func (s *PowerService) GetPowerHistory(timeRange string) ([]dto.PowerHistoryResponse, error) {
	lookback, bucketSeconds := powerHistoryWindow(timeRange)
	startTime := time.Now().Add(-lookback)

	// floor(epoch / bucket) * bucket = ปัดเวลาลงให้ตกขอบ bucket เดียวกันก่อน GROUP BY
	// (date_trunc ปัดได้แค่หน่วยสำเร็จรูป hour/day ไม่รองรับ 5 หรือ 15 นาที)
	//
	// "timestamp" ต้อง quote เพราะชนกับชื่อชนิดข้อมูลของ Postgres ส่วนที่ต้องใช้ Raw แทน
	// query builder เพราะ GORM quote อาร์กิวเมนต์ของ Group() เสมอ — GROUP BY 1 จะเพี้ยนเป็น GROUP BY "1"
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
