package handlers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"backend/internal/response"
)

type registerReq struct {
	StudentID string `json:"student_id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Password  string `json:"password" binding:"required,min=8"`
}

func Register(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req registerReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "INTERNAL", "hash ไม่สำเร็จ")
			return
		}

		var id int
		err = db.QueryRow(c.Request.Context(),
			`INSERT INTO users (student_id, name, password) VALUES ($1, $2, $3) RETURNING id`,
			req.StudentID, req.Name, string(hash),
		).Scan(&id)
		if err != nil {
			response.Error(c, http.StatusConflict, "REGISTER_FAILED", "รหัสนักศึกษานี้ถูกใช้แล้ว")
			return
		}

		response.OK(c, http.StatusCreated, gin.H{
			"id": id, "student_id": req.StudentID, "name": req.Name,
		})
	}
}

type loginReq struct {
	StudentID string `json:"student_id" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

func Login(db *pgxpool.Pool, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}

		var (
			id           int
			name, role   string
			passwordHash string
		)
		err := db.QueryRow(c.Request.Context(),
			`SELECT id, name, role, password FROM users WHERE student_id = $1`,
			req.StudentID,
		).Scan(&id, &name, &role, &passwordHash)

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("login query error: %v", err)
			response.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
		if err != nil || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
			response.Error(c, http.StatusUnauthorized, "LOGIN_FAILED", "student_id หรือ password ไม่ถูกต้อง")
			return
		}

		claims := jwt.MapClaims{
			"sub":  id,
			"role": role,
			"exp":  time.Now().Add(24 * time.Hour).Unix(),
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "INTERNAL", "สร้าง token ไม่สำเร็จ")
			return
		}

		response.OK(c, http.StatusOK, gin.H{
			"token": token,
			"user":  gin.H{"id": id, "student_id": req.StudentID, "name": name, "role": role},
		})
	}
}
