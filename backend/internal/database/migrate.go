package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // ลงทะเบียน driver "pgx" ให้ database/sql ใช้ (ไม่ต้องพึ่ง lib/pq)

	"backend/migrations"
)

// RunMigrations เรียก golang-migrate ผ่าน library ตรงๆ โดย embed ไฟล์ .sql ไว้ใน binary เลย
// เรียกได้ทุกครั้งตอน server start อย่างปลอดภัย — migration ที่ apply แล้วจะถูกข้ามอัตโนมัติ (idempotent)
func RunMigrations(dbURL string) error {
	sqlDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("open sql.DB: %w", err)
	}
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("postgres driver: %w", err)
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
