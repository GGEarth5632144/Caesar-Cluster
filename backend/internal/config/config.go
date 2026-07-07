package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	DBUrl          string
	JWTSecret      string
	FrontendOrigin string
	Provisioner    string
	ProxmoxURL     string
	ProxmoxToken   string
}

func Load() *Config {
	_ = godotenv.Load() // ไม่มีไฟล์ .env ก็ไม่ error — ใช้ env จริงของเครื่องแทน

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		DBUrl:          getEnv("DB_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		Provisioner:    getEnv("PROVISIONER", "mock"),
		ProxmoxURL:     getEnv("PROXMOX_URL", ""),
		ProxmoxToken:   getEnv("PROXMOX_TOKEN", ""),
	}
	if cfg.DBUrl == "" || cfg.JWTSecret == "" {
		log.Fatal("ต้องกำหนด DB_URL และ JWT_SECRET ใน .env")
	}
	if cfg.Provisioner == "proxmox" && (cfg.ProxmoxURL == "" || cfg.ProxmoxToken == "") {
		log.Fatal("PROVISIONER=proxmox ต้องกำหนด PROXMOX_URL และ PROXMOX_TOKEN ใน .env")
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
