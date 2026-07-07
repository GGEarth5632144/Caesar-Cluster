package services

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoCapacity = errors.New("ไม่มี node ที่มี resource เหลือเพียงพอ")

type AllocationService struct{ db *pgxpool.Pool }

func NewAllocationService(db *pgxpool.Pool) *AllocationService {
	return &AllocationService{db: db}
}

// AllocateNode: หา node ที่เหลือพอ + insert VM ใน transaction เดียวกัน
// insertVM คือ callback ที่ผู้เรียกส่งมา — จะถูกรันภายใน tx เดียวกับการล็อก node
func (s *AllocationService) AllocateNode(
	ctx context.Context, cpuCores, ramMB int,
	insertVM func(tx pgx.Tx, nodeID int) error,
) (int, error) {

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	// ถ้า Commit สำเร็จไปแล้ว Rollback ตรงนี้จะไม่มีผล — ปลอดภัยเสมอ
	defer tx.Rollback(ctx)

	var nodeID int
	err = tx.QueryRow(ctx, `
		SELECT n.id
		FROM nodes n
		WHERE n.status = 'online'
		  AND n.total_cores - COALESCE((
		        SELECT SUM(v.cpu_cores) FROM vms v
		        WHERE v.node_id = n.id
		      ), 0) >= $1
		  AND n.total_ram_mb - COALESCE((
		        SELECT SUM(v.ram_mb) FROM vms v
		        WHERE v.node_id = n.id
		      ), 0) >= $2
		ORDER BY n.id
		LIMIT 1
		FOR UPDATE OF n
	`, cpuCores, ramMB).Scan(&nodeID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNoCapacity
		}
		return 0, err
	}

	if err := insertVM(tx, nodeID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return nodeID, nil
}
