package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"ops-admin/backend/model"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

type AssetGatewayPayload struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Code         string `json:"code"`
	GatewayType  string `json:"gatewayType"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	CredentialID uint   `json:"credentialId"`
	NetworkZone  string `json:"networkZone"`
	Status       int    `json:"status"`
	Description  string `json:"description"`
}

type cleanupConn struct {
	net.Conn
	cleanup func()
}

func (c cleanupConn) Close() error {
	err := c.Conn.Close()
	if c.cleanup != nil {
		c.cleanup()
	}
	return err
}

func normalizeConnectionMode(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "gateway") {
		return "gateway"
	}
	return "direct"
}

func optionalGatewayID(connectionMode string, gatewayID uint) *uint {
	if normalizeConnectionMode(connectionMode) != "gateway" || gatewayID == 0 {
		return nil
	}
	return &gatewayID
}

func validateGatewaySelection(connectionMode string, gatewayID *uint) error {
	if normalizeConnectionMode(connectionMode) == "gateway" && (gatewayID == nil || *gatewayID == 0) {
		return errors.New("请选择访问网关")
	}
	return nil
}

func normalizeGatewayType(v string) string {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" {
		return "ssh"
	}
	return value
}

func gatewayPort(v int) int {
	if v > 0 {
		return v
	}
	return 22
}

func (s *Service) ListAssetGateways(pageNum, pageSize int, keyword string, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.AssetGateway{}).Preload("Credential")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name like ? or code like ? or host like ? or network_zone like ?", like, like, like, like)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AssetGateway
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Credential.Password = ""
		list[i].Credential.PrivateKey = ""
		list[i].Credential.Passphrase = ""
		_ = s.db.Model(&model.AssetHost{}).Where("gateway_id = ?", list[i].ID).Count(&list[i].HostCount).Error
		_ = s.db.Model(&model.AssetDatabase{}).Where("gateway_id = ?", list[i].ID).Count(&list[i].DatabaseCount).Error
		_ = s.db.Model(&model.K8sCluster{}).Where("gateway_id = ?", list[i].ID).Count(&list[i].ClusterCount).Error
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListAssetGatewayOptions() ([]model.AssetGateway, error) {
	var list []model.AssetGateway
	if err := s.db.Where("status = ?", 1).Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetAssetGateway(id uint) (*model.AssetGateway, error) {
	var item model.AssetGateway
	if err := s.db.Preload("Credential").First(&item, id).Error; err != nil {
		return nil, err
	}
	item.Credential.Password = ""
	item.Credential.PrivateKey = ""
	item.Credential.Passphrase = ""
	return &item, nil
}

func (s *Service) CreateAssetGateway(payload AssetGatewayPayload) error {
	item := model.AssetGateway{
		Name:         Trimmed(payload.Name),
		Code:         Trimmed(payload.Code),
		GatewayType:  normalizeGatewayType(payload.GatewayType),
		Host:         Trimmed(payload.Host),
		Port:         gatewayPort(payload.Port),
		CredentialID: payload.CredentialID,
		NetworkZone:  Trimmed(payload.NetworkZone),
		Status:       payload.Status,
		Description:  Trimmed(payload.Description),
	}
	if err := validateAssetGateway(item); err != nil {
		return err
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateAssetGateway(payload AssetGatewayPayload) error {
	if payload.ID == 0 {
		return errors.New("网关 ID 不能为空")
	}
	item := model.AssetGateway{
		Name:         Trimmed(payload.Name),
		Code:         Trimmed(payload.Code),
		GatewayType:  normalizeGatewayType(payload.GatewayType),
		Host:         Trimmed(payload.Host),
		Port:         gatewayPort(payload.Port),
		CredentialID: payload.CredentialID,
		NetworkZone:  Trimmed(payload.NetworkZone),
		Status:       payload.Status,
		Description:  Trimmed(payload.Description),
	}
	if err := validateAssetGateway(item); err != nil {
		return err
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Model(&model.AssetGateway{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"name":          item.Name,
		"code":          item.Code,
		"gateway_type":  item.GatewayType,
		"host":          item.Host,
		"port":          item.Port,
		"credential_id": item.CredentialID,
		"network_zone":  item.NetworkZone,
		"status":        item.Status,
		"description":   item.Description,
	}).Error
}

func (s *Service) DeleteAssetGateway(id uint) error {
	if id == 0 {
		return errors.New("网关 ID 不能为空")
	}
	var hostCount, databaseCount, clusterCount int64
	if err := s.db.Model(&model.AssetHost{}).Where("gateway_id = ?", id).Count(&hostCount).Error; err != nil {
		return err
	}
	if err := s.db.Model(&model.AssetDatabase{}).Where("gateway_id = ?", id).Count(&databaseCount).Error; err != nil {
		return err
	}
	if err := s.db.Model(&model.K8sCluster{}).Where("gateway_id = ?", id).Count(&clusterCount).Error; err != nil {
		return err
	}
	if hostCount+databaseCount+clusterCount > 0 {
		return errors.New("该网关正在被资产使用，无法删除")
	}
	return s.db.Delete(&model.AssetGateway{}, id).Error
}

func (s *Service) UpdateAssetGatewayStatus(payload AdminStatusPayload) error {
	if payload.ID == 0 {
		return errors.New("网关 ID 不能为空")
	}
	status := payload.Status
	if status != 2 {
		status = 1
	}
	return s.db.Model(&model.AssetGateway{}).Where("id = ?", payload.ID).Update("status", status).Error
}

func (s *Service) TestAssetGatewayConnection(id uint) (map[string]any, error) {
	gateway, err := s.getAssetGatewayWithCredential(id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	client, err := s.newGatewaySSHClient(*gateway)
	updates := map[string]any{"last_check_time": &now}
	if err != nil {
		updates["connect_status"] = 2
		_ = s.db.Model(&model.AssetGateway{}).Where("id = ?", id).Updates(updates).Error
		return nil, err
	}
	_ = client.Close()
	updates["connect_status"] = 1
	if err := s.db.Model(&model.AssetGateway{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return map[string]any{"connectStatus": 1, "checkedAt": now}, nil
}

func validateAssetGateway(item model.AssetGateway) error {
	if item.Name == "" {
		return errors.New("网关名称不能为空")
	}
	if item.GatewayType != "ssh" {
		return errors.New("当前仅支持 SSH 网关")
	}
	if item.Host == "" {
		return errors.New("网关地址不能为空")
	}
	if item.CredentialID == 0 {
		return errors.New("网关凭据不能为空")
	}
	return nil
}

func (s *Service) getAssetGatewayWithCredential(id uint) (*model.AssetGateway, error) {
	var gateway model.AssetGateway
	if err := s.db.Preload("Credential").First(&gateway, id).Error; err != nil {
		return nil, err
	}
	if gateway.Status == 2 {
		return nil, errors.New("网关已禁用")
	}
	if gateway.GatewayType == "" {
		gateway.GatewayType = "ssh"
	}
	if gateway.GatewayType != "ssh" {
		return nil, errors.New("当前仅支持 SSH 网关")
	}
	return &gateway, nil
}

func (s *Service) newGatewaySSHClient(gateway model.AssetGateway) (*ssh.Client, error) {
	if strings.TrimSpace(gateway.Host) == "" {
		return nil, errors.New("网关地址不能为空")
	}
	authMethod, err := credentialAuthMethod(gateway.Credential)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            gateway.Credential.Username,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", gateway.Host, gatewayPort(gateway.Port)), config)
}

func (s *Service) dialThroughGateway(ctx context.Context, gatewayID uint, network, address string) (net.Conn, func(), error) {
	conn, err := s.dialGatewayTargetContext(ctx, gatewayID, network, address)
	if err != nil {
		return nil, func() {}, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	cleanup := func() {
		_ = conn.Close()
	}
	return conn, cleanup, nil
}

func (s *Service) sharedGatewaySSHClient(gatewayID uint) (*ssh.Client, error) {
	s.gatewaySSHMu.Lock()
	defer s.gatewaySSHMu.Unlock()
	if client := s.gatewaySSHClients[gatewayID]; client != nil {
		return client, nil
	}
	gateway, err := s.getAssetGatewayWithCredential(gatewayID)
	if err != nil {
		return nil, err
	}
	client, err := s.newGatewaySSHClient(*gateway)
	if err != nil {
		return nil, err
	}
	s.gatewaySSHClients[gatewayID] = client
	return client, nil
}

func (s *Service) invalidateGatewaySSHClient(gatewayID uint, expected *ssh.Client) {
	s.gatewaySSHMu.Lock()
	defer s.gatewaySSHMu.Unlock()
	if current := s.gatewaySSHClients[gatewayID]; current == expected {
		delete(s.gatewaySSHClients, gatewayID)
		_ = current.Close()
	}
}

// dialGatewayTarget opens a multiplexed channel through the cached SSH client.
// A stale SSH transport is discarded and retried once with a new connection.
func (s *Service) dialGatewayTarget(gatewayID uint, network, address string) (net.Conn, error) {
	return s.dialGatewayTargetContext(context.Background(), gatewayID, network, address)
}

func (s *Service) dialGatewayTargetContext(ctx context.Context, gatewayID uint, network, address string) (net.Conn, error) {
	client, err := s.sharedGatewaySSHClient(gatewayID)
	if err != nil {
		return nil, err
	}
	conn, err := client.DialContext(ctx, network, address)
	if err == nil {
		return conn, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.invalidateGatewaySSHClient(gatewayID, client)
	client, reconnectErr := s.sharedGatewaySSHClient(gatewayID)
	if reconnectErr != nil {
		return nil, reconnectErr
	}
	return client.DialContext(ctx, network, address)
}

func (s *Service) startGatewayTunnel(gatewayID uint, targetAddress string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", func() {}, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			localConn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				remoteConn, err := s.dialGatewayTargetContext(dialCtx, gatewayID, "tcp", targetAddress)
				cancel()
				if err != nil {
					_ = localConn.Close()
					return
				}
				go func() {
					_, _ = io.Copy(remoteConn, localConn)
					_ = remoteConn.Close()
					_ = localConn.Close()
				}()
				go func() {
					_, _ = io.Copy(localConn, remoteConn)
					_ = remoteConn.Close()
					_ = localConn.Close()
				}()
			}()
		}
	}()
	cleanup := func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
	}
	return listener.Addr().String(), cleanup, nil
}

func gatewayReferencePreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Gateway").Preload("Gateway.Credential")
}
