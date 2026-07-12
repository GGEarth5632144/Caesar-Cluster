package test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"backend/internal/utils"
)

func TestUtils_OK_Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	utils.OK(c, 200, gin.H{"foo": "bar"})

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response ไม่ใช่ JSON ที่ถูกต้อง: %v", err)
	}
	if body["success"] != true {
		t.Errorf("success ควรเป็น true, ได้ %v แทน", body["success"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok || data["foo"] != "bar" {
		t.Errorf("data payload ไม่ตรงกับที่ส่งเข้าไป: %v", body["data"])
	}
}

func TestUtils_Error_Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	utils.Error(c, 400, "INVALID_INPUT", "ทดสอบ")

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response ไม่ใช่ JSON ที่ถูกต้อง: %v", err)
	}
	if body["success"] != false {
		t.Errorf("success ควรเป็น false, ได้ %v แทน", body["success"])
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["code"] != "INVALID_INPUT" {
		t.Errorf("error.code ไม่ตรง: %v", body["error"])
	}
}
