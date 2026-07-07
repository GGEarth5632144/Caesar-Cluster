package services

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type ProxmoxProvisioner struct {
	baseURL string // เช่น https://10.x.x.x:8006/api2/json
	token   string // PVEAPIToken=user@realm!tokenid=uuid
	client  *http.Client
}

func NewProxmoxProvisioner(baseURL, token string) *ProxmoxProvisioner {
	return &ProxmoxProvisioner{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *ProxmoxProvisioner) CreateVM(ctx context.Context, nodeHostname, vmName string, cores, ramMB int) error {
	// TODO: POST {baseURL}/nodes/{nodeHostname}/qemu
	// Header: Authorization: PVEAPIToken=...
	return fmt.Errorf("proxmox provisioner: not implemented yet")
}

func (p *ProxmoxProvisioner) DeleteVM(ctx context.Context, nodeHostname string, vmID int) error {
	// TODO: DELETE {baseURL}/nodes/{nodeHostname}/qemu/{vmID}
	return fmt.Errorf("proxmox provisioner: not implemented yet")
}
