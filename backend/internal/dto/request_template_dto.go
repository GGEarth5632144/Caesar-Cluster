package dto

type CreateRequestTemplateRequest struct {
	OptionName      string `json:"option_name" binding:"required"`
	Category        string `json:"category" binding:"required"`
	Description     string `json:"description" binding:"required"`
	RelateSubject   string `json:"relate_subject" binding:"required"`
	CPULimitMilli   int    `json:"cpu_limit_milli" binding:"required,gt=0"`
	RAMLimitMB      int    `json:"ram_limit_mb" binding:"required,gt=0"`
	StorageGB       int    `json:"storage_gb" binding:"required,gt=0"`
}

// UpdateRequestTemplateRequest สำหรับรับข้อมูลแก้ไข (ทุกฟิลด์เป็น Pointer เพื่อรองรับ Partial Update)
type UpdateRequestTemplateRequest struct {
	OptionName    *string `json:"option_name"`
	Category      *string `json:"category"`
	Description   *string `json:"description"`
	RelateSubject *string `json:"relate_subject"`
	CPULimitMilli *int    `json:"cpu_limit_milli" binding:"omitempty,gt=0"`
	RAMLimitMB    *int    `json:"ram_limit_mb" binding:"omitempty,gt=0"`
	StorageGB     *int    `json:"storage_gb" binding:"omitempty,gt=0"`
	IsActive      *bool   `json:"is_active"`
}
