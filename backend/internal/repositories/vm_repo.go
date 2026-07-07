package repositories

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/models"
)

type VMRepo struct{ db *pgxpool.Pool }

func NewVMRepo(db *pgxpool.Pool) *VMRepo { return &VMRepo{db: db} }

func (r *VMRepo) ListByOwner(ctx context.Context, ownerID int) ([]models.VM, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, owner_id, node_id, name, cpu_cores, ram_mb, status, created_at
		 FROM vms WHERE owner_id = $1 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vms := []models.VM{}
	for rows.Next() {
		var v models.VM
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.NodeID, &v.Name,
			&v.CPUCores, &v.RAMMB, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		vms = append(vms, v)
	}
	return vms, rows.Err()
}

func (r *VMRepo) Delete(ctx context.Context, id, ownerID int) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM vms WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
