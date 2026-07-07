package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/response"
)

type createNodeReq struct {
	Hostname   string `json:"hostname" binding:"required"`
	TotalCores int    `json:"total_cores" binding:"required,min=1"`
	TotalRAMMB int    `json:"total_ram_mb" binding:"required,min=512"`
}

func CreateNode(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createNodeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		var id int
		err := db.QueryRow(c.Request.Context(),
			`INSERT INTO nodes (hostname, total_cores, total_ram_mb) VALUES ($1, $2, $3) RETURNING id`,
			req.Hostname, req.TotalCores, req.TotalRAMMB).Scan(&id)
		if err != nil {
			response.Error(c, http.StatusConflict, "NODE_EXISTS", "hostname นี้มีอยู่แล้ว")
			return
		}
		response.OK(c, http.StatusCreated, gin.H{"id": id, "hostname": req.Hostname})
	}
}

// ListNodes — โชว์การใช้งานจริงของแต่ละ node (used vs total)
func ListNodes(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(c.Request.Context(), `
			SELECT n.id, n.hostname, n.status, n.total_cores, n.total_ram_mb,
			       COALESCE(SUM(v.cpu_cores), 0) AS used_cores,
			       COALESCE(SUM(v.ram_mb), 0)    AS used_ram_mb
			FROM nodes n
			LEFT JOIN vms v ON v.node_id = n.id
			GROUP BY n.id ORDER BY n.id`)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
			return
		}
		defer rows.Close()

		type nodeUsage struct {
			ID         int    `json:"id"`
			Hostname   string `json:"hostname"`
			Status     string `json:"status"`
			TotalCores int    `json:"total_cores"`
			TotalRAMMB int    `json:"total_ram_mb"`
			UsedCores  int    `json:"used_cores"`
			UsedRAMMB  int    `json:"used_ram_mb"`
		}
		list := []nodeUsage{}
		for rows.Next() {
			var n nodeUsage
			if err := rows.Scan(&n.ID, &n.Hostname, &n.Status,
				&n.TotalCores, &n.TotalRAMMB, &n.UsedCores, &n.UsedRAMMB); err != nil {
				response.Error(c, http.StatusInternalServerError, "INTERNAL", "เกิดข้อผิดพลาด")
				return
			}
			list = append(list, n)
		}
		response.OK(c, http.StatusOK, list)
	}
}
