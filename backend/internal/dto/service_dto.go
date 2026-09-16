package dto

// CreateServiceRequest = body ของ POST /api/services — ขอ deploy workload เข้า namespace ของตัวเอง
//
// เลือกสเปกได้ 2 ทาง:
//  1. ส่ง request_template_id (เลือกจาก "choices" ที่ admin สร้างไว้) → ระบบใช้ cpu/ram ของ template นั้น
//  2. ไม่ส่ง request_template_id แต่กรอก cpu_milli / ram_mb เองตามต้องการ
//
// เพดานใน binding เป็นเพดานของ service 1 ตัว (300% = 3000m, 2 GB)
// ส่วนโควตารวมทั้ง namespace ถูกเช็คอีกชั้นใน QuotaService (binding ตรงนี้ไม่รู้จักโควตา)
// data flow: JSON จาก client → ServiceController.Create → services.CreateServiceParams → ServiceManager.Create
type CreateServiceRequest struct {
	Name              string `json:"name" binding:"required,min=3,max=50"`
	Image             string `json:"image" binding:"required,min=3,max=200"`
	RequestTemplateID *int   `json:"request_template_id" binding:"omitempty,min=1"`
	CPUMilli          int    `json:"cpu_milli" binding:"required_without=RequestTemplateID,omitempty,min=100,max=3000"`
	RAMMB             int    `json:"ram_mb" binding:"required_without=RequestTemplateID,omitempty,min=128,max=2048"`
	// ContainerPort ไม่ใช่ NodePort — คนละชั้นกัน (ดู entity/service.go)
	// ทั้งคู่ omitempty: ไม่ส่งมา = ServiceManager เติม default ให้ (8080 / 1 replica)
	ContainerPort int `json:"container_port" binding:"omitempty,min=1,max=65535"`
	// หักโควตา namespace เป็น cpu_milli × replicas (เช็คใน QuotaService)
	Replicas int `json:"replicas" binding:"omitempty,min=1,max=10"`
	// EnvVars ไม่บังคับ — เช็ครูปแบบ/จำนวนแยกใน controller (isValidEnvVars) แทนการใช้ binding tag
	// เพราะ go-playground/validator เช็ค map ได้จำกัด (ไม่มี built-in ตรวจ key pattern ของแต่ละ entry)
	EnvVars map[string]string `json:"env_vars"`

	// IsDatabase ถูกถอดแล้ว — database สร้างผ่าน POST /api/databases (template) เท่านั้น (docs 029)
	// ยังรับ field ไว้เพื่อตอบ 400 ให้ client เก่าที่ส่ง true มา แทนการสร้าง web ธรรมดาให้เงียบๆ
	IsDatabase bool `json:"is_database"`

	// DataPath = ตำแหน่งที่ image เก็บข้อมูล ใช้เป็นจุด mount ของ PVC
	// บังคับส่งทุกครั้งที่ขอดิสก์ (web ที่ส่ง storage_mb มา) กติกาอยู่ใน services.ValidateDataPath
	DataPath string `json:"data_path" binding:"omitempty,max=200"`

	// StorageMB = ขนาดดิสก์ที่ขอ เป็น MB เสมอ — ส่งมา (หรือส่ง data_path มา) = ขอดิสก์ถาวร ได้ทั้ง database และ web
	// ไม่ส่งแต่ขอดิสก์ใช้ entity.DefaultStorageMBPerService · เพดานต้องตรงกับ entity.MaxStorageMBPerService
	StorageMB int `json:"storage_mb" binding:"omitempty,min=1024,max=20480"`
}

// CreateDatabaseRequest = body ของ POST /api/databases — deploy database จาก template (docs 029)
//
// image/พอร์ต/จุด mount/env มาจาก catalog ใน backend ทั้งหมด ผู้ใช้กำหนดแค่ engine/version + credential
// กติกาของ username/password/database ตรวจใน services (validateDatabaseCredentials) ที่เดียว
// RAM ขั้นต่ำต่อ engine ตรวจใน ServiceManager.CreateDatabase (binding ไม่รู้จัก engine)
type CreateDatabaseRequest struct {
	Engine            string `json:"engine" binding:"required,oneof=postgresql mysql mariadb"`
	Version           string `json:"version" binding:"omitempty,max=20"`
	Name              string `json:"name" binding:"required,min=3,max=50"`
	Username          string `json:"username" binding:"required,max=32"`
	Password          string `json:"password" binding:"required,max=64"`
	Database          string `json:"database" binding:"required,max=63"`
	StorageMB         int    `json:"storage_mb" binding:"omitempty,min=1024,max=20480"`
	RequestTemplateID *int   `json:"request_template_id" binding:"omitempty,min=1"`
	CPUMilli          int    `json:"cpu_milli" binding:"required_without=RequestTemplateID,omitempty,min=100,max=3000"`
	RAMMB             int    `json:"ram_mb" binding:"required_without=RequestTemplateID,omitempty,min=128,max=2048"`
}

// ScaleServiceRequest = body ของ PATCH /api/services/:id/scale — ปรับจำนวน Pod ของ service ที่ deploy แล้ว
// data flow: ServiceController.Scale → ServiceManager.Scale → QuotaService.ReserveScale (เช็คโควตาก่อน UPDATE)
type ScaleServiceRequest struct {
	Replicas int `json:"replicas" binding:"required,min=1,max=10"`
}


type UpdateServiceRequest struct {
    Name          string `json:"name" binding:"required,min=3,max=50"`
    
    Image         string `json:"image" binding:"required,min=3,max=200"`

    CPUMilli      int    `json:"cpu_milli" binding:"required,min=100,max=3000"`
    RAMMB         int    `json:"ram_mb" binding:"required,min=128,max=2048"`
    
    ContainerPort int    `json:"container_port" binding:"required,min=1,max=65535"`
    Replicas      int    `json:"replicas" binding:"required,min=1,max=10"`
    EnvVars       map[string]string `json:"env_vars"`
    IsDatabase    bool   `json:"is_database"`
    DataPath      string `json:"data_path" binding:"omitempty,max=200"`
    StorageMB     int    `json:"storage_mb" binding:"omitempty,min=1024,max=20480"`
}
// DeleteServiceRequest = body ของการลบ/ตั้งเวลาลบ service ฝั่ง admin — reason ส่งไปในอีเมลแจ้งสมาชิก
type DeleteServiceRequest struct {
	Reason string `json:"reason" binding:"required,min=1,max=1000"`
}
