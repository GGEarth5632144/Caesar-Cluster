package services

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/models"
	"backend/internal/repositories"
)

type VMService struct {
	db    *pgxpool.Pool
	repo  *repositories.VMRepo
	alloc *AllocationService
	prov  Provisioner
}

func NewVMService(db *pgxpool.Pool, repo *repositories.VMRepo,
	alloc *AllocationService, prov Provisioner) *VMService {
	return &VMService{db: db, repo: repo, alloc: alloc, prov: prov}
}

func (s *VMService) ListByOwner(ctx context.Context, ownerID int) ([]models.VM, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

func (s *VMService) Create(ctx context.Context, ownerID int, req models.CreateVMRequest) (*models.VM, error) {
	vm := &models.VM{
		OwnerID:  ownerID,
		Name:     req.Name,
		CPUCores: req.CPUCores,
		RAMMB:    req.RAMMB,
		Status:   "creating",
	}

	nodeID, err := s.alloc.AllocateNode(ctx, req.CPUCores, req.RAMMB,
		func(tx pgx.Tx, nodeID int) error {
			return tx.QueryRow(ctx,
				`INSERT INTO vms (owner_id, node_id, name, cpu_cores, ram_mb, status)
				 VALUES ($1, $2, $3, $4, $5, 'creating')
				 RETURNING id, created_at`,
				ownerID, nodeID, req.Name, req.CPUCores, req.RAMMB,
			).Scan(&vm.ID, &vm.CreatedAt)
		})
	if err != nil {
		return nil, err
	}

	vm.NodeID = nodeID

	// หา hostname ของ node เพื่อส่งให้ provisioner
	var hostname string
	if err := s.db.QueryRow(ctx,
		`SELECT hostname FROM nodes WHERE id = $1`, nodeID).Scan(&hostname); err != nil {
		return nil, err
	}

	// สั่งสร้าง VM จริง (mock ตอนนี้ / proxmox ในอนาคต)
	if err := s.prov.CreateVM(ctx, hostname, vm.Name, vm.CPUCores, vm.RAMMB); err != nil {
		// สร้างจริงไม่สำเร็จ → mark failed ใน DB (แถวยังอยู่ให้ตรวจสอบได้)
		_, _ = s.db.Exec(ctx, `UPDATE vms SET status = 'failed' WHERE id = $1`, vm.ID)
		return nil, err
	}

	// สำเร็จ → running
	if _, err := s.db.Exec(ctx,
		`UPDATE vms SET status = 'running' WHERE id = $1`, vm.ID); err != nil {
		return nil, err
	}
	vm.Status = "running"
	return vm, nil

}

func (s *VMService) Delete(ctx context.Context, id, ownerID int) (bool, error) {
	var nodeID int
	err := s.db.QueryRow(ctx,
		`SELECT node_id FROM vms WHERE id = $1 AND owner_id = $2`, id, ownerID).Scan(&nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	var hostname string
	if err := s.db.QueryRow(ctx,
		`SELECT hostname FROM nodes WHERE id = $1`, nodeID).Scan(&hostname); err != nil {
		return false, err
	}

	// สั่ง deprovision VM จริงก่อน แล้วค่อยลบ row — กันไม่ให้ VM ค้างอยู่บน node โดยไม่มี record
	if err := s.prov.DeleteVM(ctx, hostname, id); err != nil {
		return false, err
	}

	return s.repo.Delete(ctx, id, ownerID)
}
