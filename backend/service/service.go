package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
	"ops-admin/backend/auth"
	"ops-admin/backend/internal/domain/dnsserver"
	"ops-admin/backend/model"
	"ops-admin/backend/util"
)

type Service struct {
	db                   *gorm.DB
	opsScheduler         *OpsScheduler
	opsSchedulerOnce     sync.Once
	monitorScheduler     *MonitorScheduler
	monitorSchedulerOnce sync.Once
	dbBackupScheduler    *DatabaseBackupScheduler
	dbBackupOnce         sync.Once
	finOpsScheduler      *FinOpsScheduler
	finOpsSchedulerOnce  sync.Once
	notifyDispatcherOnce sync.Once
	notifyConcurrency    chan struct{}
	dnsManager           *dnsserver.Manager
	certificateConfig    CertificateRuntimeConfig
	certificateOnce      sync.Once
	monitorNotifyMu      sync.Mutex
	// Gateway SSH connections are multiplexed by ssh.Client. Keeping one client
	// per gateway avoids repeating the public-network SSH handshake on every
	// Kubernetes API request.
	gatewaySSHMu      sync.Mutex
	gatewaySSHClients map[uint]*ssh.Client
	// Cluster overview is relatively expensive for gateway clusters. A brief
	// cache avoids duplicate page-load requests while singleflight coalesces
	// concurrent refreshes for the same cluster.
	k8sOverviewMu    sync.Mutex
	k8sOverviewCache map[uint]k8sOverviewCacheEntry
	k8sOverviewGroup singleflight.Group
}

type k8sOverviewCacheEntry struct {
	detail    model.K8sClusterDetail
	expiresAt time.Time
}

type AssetTerminalSession struct {
	Host    model.AssetHost
	Client  *ssh.Client
	Session *ssh.Session
	Stdin   io.WriteCloser
	Stdout  io.Reader
	Stderr  io.Reader
}

func New(db *gorm.DB) *Service {
	svc := &Service{db: db, gatewaySSHClients: make(map[uint]*ssh.Client), k8sOverviewCache: make(map[uint]k8sOverviewCacheEntry), certificateConfig: defaultCertificateRuntimeConfig()}
	svc.dnsManager = dnsserver.NewManager(db)
	svc.ensureDefaultEnvironments()
	svc.initOpsScheduler()
	svc.initMonitorScheduler()
	svc.initDatabaseBackupScheduler()
	svc.initFinOpsScheduler()
	svc.initNotifyDispatcher()
	return svc
}

type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	RememberLogin bool   `json:"rememberLogin"`
}

type AdminPayload struct {
	ID       uint   `json:"id"`
	PostID   uint   `json:"postId"`
	RoleID   uint   `json:"roleId"`
	DeptID   uint   `json:"deptId"`
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Status   int    `json:"status"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Note     string `json:"note"`
}

type RolePayload struct {
	ID          uint   `json:"id"`
	RoleName    string `json:"roleName"`
	RoleKey     string `json:"roleKey"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type MenuPayload struct {
	ID         uint   `json:"id"`
	ParentID   uint   `json:"parentId"`
	MenuName   string `json:"menuName"`
	Icon       string `json:"icon"`
	Value      string `json:"value"`
	MenuType   int    `json:"menuType"`
	URL        string `json:"url"`
	MenuStatus int    `json:"menuStatus"`
	Sort       int    `json:"sort"`
}

type DeptPayload struct {
	ID         uint   `json:"id"`
	ParentID   uint   `json:"parentId"`
	DeptType   int    `json:"deptType"`
	DeptName   string `json:"deptName"`
	DeptStatus int    `json:"deptStatus"`
}

type PostPayload struct {
	ID         uint   `json:"id"`
	PostCode   string `json:"postCode"`
	PostName   string `json:"postName"`
	PostStatus int    `json:"postStatus"`
	Remark     string `json:"remark"`
}

type RoleMenuPayload struct {
	ID      uint   `json:"id"`
	MenuIDs []uint `json:"menuIds"`
}

type IDPayload struct {
	ID uint `json:"id"`
}

type BatchIDPayload struct {
	IDs []uint `json:"ids"`
}

type AssetHostBatchCredentialPayload struct {
	IDs          []uint `json:"ids"`
	CredentialID uint   `json:"credentialId"`
}

type AssetHostGroupMemberPayload struct {
	HostID  uint   `json:"hostId"`
	HostIDs []uint `json:"hostIds"`
	GroupID uint   `json:"groupId"`
}

type AdminStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type PostStatusPayload struct {
	ID         uint `json:"id"`
	PostStatus int  `json:"postStatus"`
}

type RoleStatusPayload struct {
	ID     uint `json:"id"`
	Status int  `json:"status"`
}

type SystemConfigPayload struct {
	SiteName             string `json:"siteName"`
	SiteSlogan           string `json:"siteSlogan"`
	LogoType             string `json:"logoType"`
	LogoValue            string `json:"logoValue"`
	LoginTitle           string `json:"loginTitle"`
	LoginSubtitle        string `json:"loginSubtitle"`
	UseLoginBackground   bool   `json:"useLoginBackground"`
	LoginBackground      string `json:"loginBackground"`
	PrimaryColor         string `json:"primaryColor"`
	SidebarTheme         string `json:"sidebarTheme"`
	RememberLoginEnabled bool   `json:"rememberLoginEnabled"`
}

type AssetHostPayload struct {
	ID             uint   `json:"id"`
	HostName       string `json:"hostName"`
	Alias          string `json:"alias"`
	GroupID        uint   `json:"groupId"`
	GroupIDs       []uint `json:"groupIds"`
	CredentialID   uint   `json:"credentialId"`
	ConnectionMode string `json:"connectionMode"`
	GatewayID      uint   `json:"gatewayId"`
	CloudAccountID uint   `json:"cloudAccountId"`
	PrivateIP      string `json:"privateIp"`
	PublicIP       string `json:"publicIp"`
	SSHUser        string `json:"sshUser"`
	SSHIP          string `json:"sshIp"`
	SSHPort        int    `json:"sshPort"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CPU            string `json:"cpu"`
	Memory         string `json:"memory"`
	Disk           string `json:"disk"`
	Environment    string `json:"environment"`
	Provider       string `json:"provider"`
	Region         string `json:"region"`
	Status         int    `json:"status"`
	Description    string `json:"description"`
	Operator       string `json:"-"`
}

type AssetHostImportRow struct {
	HostName       string `json:"hostName"`
	SSHIP          string `json:"sshIp"`
	SSHPort        int    `json:"sshPort"`
	SSHUser        string `json:"sshUser"`
	CredentialName string `json:"credentialName"`
	ConnectionMode string `json:"connectionMode"`
	GatewayName    string `json:"gatewayName"`
	Environment    string `json:"environment"`
	PrivateIP      string `json:"privateIp"`
	PublicIP       string `json:"publicIp"`
	Provider       string `json:"provider"`
	Region         string `json:"region"`
	Description    string `json:"description"`
}

type AssetCloudSyncPayload struct {
	GroupID            uint     `json:"groupId"`
	Provider           string   `json:"provider"`
	CredentialID       uint     `json:"credentialId"`
	ConnectionMode     string   `json:"connectionMode"`
	GatewayID          uint     `json:"gatewayId"`
	Environment        string   `json:"environment"`
	Regions            []string `json:"regions"`
	Region             string   `json:"region"`
	CloudAccountID     uint     `json:"cloudAccountId"`
	UseExistingAccount bool     `json:"useExistingAccount"`
	AccessKey          string   `json:"accessKey"`
	SecretKey          string   `json:"secretKey"`
	AccountName        string   `json:"accountName"`
	SaveAccount        bool     `json:"saveAccount"`
}

type AssetHostGroupPayload struct {
	ID          uint   `json:"id"`
	ParentID    uint   `json:"parentId"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Sort        int    `json:"sort"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type AssetCredentialPayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	AuthType    string `json:"authType"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	PrivateKey  string `json:"privateKey"`
	Passphrase  string `json:"passphrase"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type AssetCloudAccountPayload struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	AccessKey   string   `json:"accessKey"`
	SecretKey   string   `json:"secretKey"`
	Regions     []string `json:"regions"`
	Region      string   `json:"region"`
	Status      int      `json:"status"`
	Description string   `json:"description"`
}

type AssetDatabasePayload struct {
	ID             uint     `json:"id"`
	Name           string   `json:"name"`
	DBType         string   `json:"dbType"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	Password       string   `json:"password"`
	ConnectionMode string   `json:"connectionMode"`
	GatewayID      uint     `json:"gatewayId"`
	DBName         string   `json:"dbName"`
	Charset        string   `json:"charset"`
	Env            string   `json:"env"`
	Tags           []string `json:"tags"`
	AccessMode     string   `json:"accessMode"`
	MonitorEnabled bool     `json:"monitorEnabled"`
	Status         int      `json:"status"`
	Description    string   `json:"description"`
	Operator       string   `json:"-"`
}

type AssetHostGroupNode struct {
	ID          uint                  `json:"id"`
	ParentID    uint                  `json:"parentId"`
	Name        string                `json:"name"`
	Code        string                `json:"code"`
	Sort        int                   `json:"sort"`
	Status      int                   `json:"status"`
	Description string                `json:"description"`
	HostCount   int64                 `json:"hostCount"`
	CreatedAt   time.Time             `json:"createTime"`
	UpdatedAt   time.Time             `json:"updateTime"`
	Children    []*AssetHostGroupNode `json:"children,omitempty"`
}

type AssetHostGroupListResult struct {
	List  []AssetHostGroupNode  `json:"list"`
	Tree  []*AssetHostGroupNode `json:"tree"`
	Total int                   `json:"total"`
}

type profileRow struct {
	model.Admin
	RoleName string `gorm:"column:role_name"`
	DeptName string `gorm:"column:dept_name"`
	PostName string `gorm:"column:post_name"`
}

func (s *Service) Login(req LoginRequest, ip string, browser string, osName string) (map[string]any, string, error) {
	var admin model.Admin
	if err := s.db.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		s.createLoginLog(req.Username, ip, browser, osName, 2, "用户名或密码错误")
		return nil, "", errors.New("用户名或密码错误")
	}
	if admin.Status != 1 {
		s.createLoginLog(req.Username, ip, browser, osName, 2, "账号已停用")
		return nil, "", errors.New("账号已停用")
	}
	if !util.CheckPassword(admin.Password, req.Password) {
		s.createLoginLog(req.Username, ip, browser, osName, 2, "用户名或密码错误")
		return nil, "", errors.New("用户名或密码错误")
	}

	sessionID, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, "", err
	}
	refreshToken, err := auth.NewOpaqueToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	config, err := s.GetSystemConfig()
	if err != nil {
		return nil, "", err
	}
	rememberLogin := req.RememberLogin && config.RememberLoginEnabled
	session := model.AuthSession{
		ID:               sessionID,
		AdminID:          admin.ID,
		RefreshTokenHash: auth.HashOpaqueToken(refreshToken),
		LastActivityAt:   now,
		ExpiresAt:        now.Add(auth.SessionMaxTTL),
		RememberLogin:    rememberLogin,
	}
	if err := s.db.Create(&session).Error; err != nil {
		return nil, "", err
	}
	token, accessExpiresAt, err := auth.GenerateTokenUntil(admin.ID, admin.Username, session.ID, session.ExpiresAt)
	if err != nil {
		_ = s.db.Delete(&session).Error
		return nil, "", err
	}
	s.createLoginLog(req.Username, ip, browser, osName, 1, "登录成功")

	roleID := s.getRoleID(admin.ID)
	return map[string]any{
		"token":                token,
		"accessTokenExpiresAt": accessExpiresAt.UnixMilli(),
		"sessionExpiresAt":     session.ExpiresAt.UnixMilli(),
		"rememberLogin":        session.RememberLogin,
		"sysAdmin":             admin,
		"leftMenuList":         s.CurrentMenus(roleID),
		"permissionList":       s.CurrentPermissions(roleID),
		"systemConfig":         config,
	}, refreshToken, nil
}

func (s *Service) RefreshSession(refreshToken string, reportedActivityAt time.Time) (map[string]any, string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, "", errors.New("刷新凭证缺失")
	}
	var session model.AuthSession
	if err := s.db.Where("refresh_token_hash = ?", auth.HashOpaqueToken(refreshToken)).First(&session).Error; err != nil {
		return nil, "", errors.New("登录会话无效")
	}
	now := time.Now()
	lastActivityAt := session.LastActivityAt
	if reportedActivityAt.After(lastActivityAt) && !reportedActivityAt.After(now.Add(time.Minute)) {
		lastActivityAt = reportedActivityAt
	}
	if session.RevokedAt != nil || auth.SessionExpired(now, lastActivityAt, session.ExpiresAt, session.RememberLogin) {
		return nil, "", errors.New("登录已过期")
	}

	var admin model.Admin
	if err := s.db.First(&admin, session.AdminID).Error; err != nil || admin.Status != 1 {
		return nil, "", errors.New("账号不可用")
	}
	if err := s.db.Model(&model.AuthSession{}).Where("id = ? AND revoked_at IS NULL", session.ID).Updates(map[string]any{
		"last_activity_at": lastActivityAt,
	}).Error; err != nil {
		return nil, "", err
	}
	accessToken, accessExpiresAt, err := auth.GenerateTokenUntil(admin.ID, admin.Username, session.ID, session.ExpiresAt)
	if err != nil {
		return nil, "", err
	}
	return map[string]any{
		"token":                accessToken,
		"accessTokenExpiresAt": accessExpiresAt.UnixMilli(),
		"sessionExpiresAt":     session.ExpiresAt.UnixMilli(),
		"rememberLogin":        session.RememberLogin,
	}, refreshToken, nil
}

func (s *Service) RevokeSession(refreshToken string) {
	if strings.TrimSpace(refreshToken) == "" {
		return
	}
	now := time.Now()
	_ = s.db.Model(&model.AuthSession{}).
		Where("refresh_token_hash = ? AND revoked_at IS NULL", auth.HashOpaqueToken(refreshToken)).
		Update("revoked_at", &now).Error
}

func (s *Service) GetProfile(userID uint) (map[string]any, error) {
	user, err := s.profileUser(userID)
	if err != nil {
		return nil, err
	}
	roleID := s.getRoleID(userID)
	config, _ := s.GetSystemConfig()
	return map[string]any{
		"user":           user,
		"leftMenuList":   s.CurrentMenus(roleID),
		"permissionList": s.CurrentPermissions(roleID),
		"systemConfig":   config,
	}, nil
}

func (s *Service) GetSystemConfig() (model.SystemConfig, error) {
	return s.ensureSystemConfig()
}

func (s *Service) UpdateSystemConfig(payload SystemConfigPayload) (model.SystemConfig, error) {
	cfg, err := s.ensureSystemConfig()
	if err != nil {
		return cfg, err
	}

	updates := map[string]any{
		"site_name":              Trimmed(payload.SiteName),
		"site_slogan":            Trimmed(payload.SiteSlogan),
		"logo_type":              Trimmed(payload.LogoType),
		"logo_value":             Trimmed(payload.LogoValue),
		"login_title":            Trimmed(payload.LoginTitle),
		"login_subtitle":         Trimmed(payload.LoginSubtitle),
		"use_login_background":   payload.UseLoginBackground,
		"login_background":       Trimmed(payload.LoginBackground),
		"primary_color":          Trimmed(payload.PrimaryColor),
		"sidebar_theme":          Trimmed(payload.SidebarTheme),
		"remember_login_enabled": payload.RememberLoginEnabled,
	}

	if updates["site_name"] == "" {
		updates["site_name"] = "Ops Admin"
	}
	if updates["logo_type"] == "" {
		updates["logo_type"] = "text"
	}
	if updates["primary_color"] == "" {
		updates["primary_color"] = "#5b6cf9"
	}
	if updates["sidebar_theme"] == "" {
		updates["sidebar_theme"] = "dark"
	}

	if err := s.db.Model(&cfg).Updates(updates).Error; err != nil {
		return cfg, err
	}
	return s.ensureSystemConfig()
}

func (s *Service) createLoginLog(username, ip, browser, osName string, status int, message string) {
	entry := model.LoginLog{
		Username:      username,
		IPAddress:     ip,
		LoginLocation: "local",
		Browser:       normalizeBrowser(browser),
		OS:            normalizeOS(browser, osName),
		LoginStatus:   status,
		Message:       message,
		LoginTime:     time.Now(),
	}
	if err := s.db.Create(&entry).Error; err != nil {
		log.Printf("create login log failed: %v", err)
	}
}

func (s *Service) ensureSystemConfig() (model.SystemConfig, error) {
	var cfg model.SystemConfig
	err := s.db.First(&cfg).Error
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return cfg, err
	}

	cfg = model.SystemConfig{
		SiteName:             "Ops Admin",
		SiteSlogan:           "个人运维管理平台",
		LogoType:             "text",
		LogoValue:            "OA",
		LoginTitle:           "Ops Admin",
		LoginSubtitle:        "系统管理与运维控制台",
		UseLoginBackground:   false,
		PrimaryColor:         "#5b6cf9",
		SidebarTheme:         "dark",
		RememberLoginEnabled: true,
	}
	return cfg, s.db.Create(&cfg).Error
}

func (s *Service) getRoleID(adminID uint) uint {
	var adminRole model.AdminRole
	s.db.Where("admin_id = ?", adminID).First(&adminRole)
	return adminRole.RoleID
}

func (s *Service) CurrentPermissions(roleID uint) []string {
	var values []string
	s.db.Table("sys_menu").
		Select("sys_menu.value").
		Joins("join sys_role_menu on sys_role_menu.menu_id = sys_menu.id").
		Where("sys_role_menu.role_id = ? and sys_menu.value <> ''", roleID).
		Order("sys_menu.sort asc").
		Scan(&values)
	return values
}

func (s *Service) CurrentMenus(roleID uint) []map[string]any {
	var menus []model.Menu
	s.db.Table("sys_menu").
		Joins("join sys_role_menu on sys_role_menu.menu_id = sys_menu.id").
		Where("sys_role_menu.role_id = ? and sys_menu.menu_status = ? and sys_menu.menu_type in ?", roleID, 1, []int{1, 2}).
		Order("sys_menu.sort asc, sys_menu.id asc").
		Find(&menus)

	parentMap := map[uint][]model.Menu{}
	roots := make([]model.Menu, 0)
	applicationRoots := map[string]bool{
		"/assets":       true,
		"/ops":          true,
		"/applications": true,
		"/notify":       true,
		"/monitor":      true,
	}
	for _, menu := range menus {
		if menu.ParentID == 0 {
			// Application navigation is rendered by the frontend app switcher.
			// Keep these menus in sys_menu for role assignment, but never expose
			// them as children of the console sidebar.
			if applicationRoots[menu.URL] {
				continue
			}
			roots = append(roots, menu)
			continue
		}
		parentMap[menu.ParentID] = append(parentMap[menu.ParentID], menu)
	}

	result := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		item := map[string]any{
			"id":       root.ID,
			"menuName": root.MenuName,
			"icon":     root.Icon,
			"url":      root.URL,
		}
		children := make([]map[string]any, 0)
		for _, child := range parentMap[root.ID] {
			children = append(children, map[string]any{
				"id":       child.ID,
				"menuName": child.MenuName,
				"icon":     child.Icon,
				"url":      child.URL,
			})
		}
		item["menuSvoList"] = children
		result = append(result, item)
	}
	return result
}

func (s *Service) profileUser(userID uint) (map[string]any, error) {
	var row profileRow
	err := s.db.Table("sys_admin").
		Select("sys_admin.*, sys_role.role_name, sys_dept.dept_name, sys_post.post_name").
		Joins("left join sys_admin_role on sys_admin_role.admin_id = sys_admin.id").
		Joins("left join sys_role on sys_role.id = sys_admin_role.role_id").
		Joins("left join sys_dept on sys_dept.id = sys_admin.dept_id").
		Joins("left join sys_post on sys_post.id = sys_admin.post_id").
		Where("sys_admin.id = ?", userID).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":         row.ID,
		"username":   row.Username,
		"nickname":   row.Nickname,
		"status":     row.Status,
		"email":      row.Email,
		"phone":      row.Phone,
		"note":       row.Note,
		"deptId":     row.DeptID,
		"postId":     row.PostID,
		"deptName":   row.DeptName,
		"postName":   row.PostName,
		"roleName":   row.RoleName,
		"createTime": row.CreatedAt,
		"updateTime": row.UpdatedAt,
	}, nil
}

func (s *Service) ListAdmins(pageNum, pageSize int, username string, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	query := s.db.Table("sys_admin").
		Select("sys_admin.id, sys_admin.username, sys_admin.nickname, sys_admin.status, sys_admin.post_id, sys_admin.dept_id, sys_admin.email, sys_admin.phone, sys_admin.note, sys_admin.created_at, sys_post.post_name, sys_dept.dept_name, sys_role.role_name, sys_admin_role.role_id").
		Joins("left join sys_post on sys_post.id = sys_admin.post_id").
		Joins("left join sys_dept on sys_dept.id = sys_admin.dept_id").
		Joins("left join sys_admin_role on sys_admin_role.admin_id = sys_admin.id").
		Joins("left join sys_role on sys_role.id = sys_admin_role.role_id")

	if username != "" {
		query = query.Where("sys_admin.username like ?", "%"+username+"%")
	}
	if status != "" {
		query = query.Where("sys_admin.status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var list []model.AdminListItem
	if err := query.Order("sys_admin.id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Scan(&list).Error; err != nil {
		return nil, err
	}

	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetAdmin(id uint) (map[string]any, error) {
	var admin model.Admin
	if err := s.db.First(&admin, id).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"id":       admin.ID,
		"postId":   admin.PostID,
		"deptId":   admin.DeptID,
		"username": admin.Username,
		"nickname": admin.Nickname,
		"status":   admin.Status,
		"email":    admin.Email,
		"phone":    admin.Phone,
		"note":     admin.Note,
		"roleId":   s.getRoleID(admin.ID),
	}, nil
}

func (s *Service) CreateAdmin(payload AdminPayload) error {
	var count int64
	s.db.Model(&model.Admin{}).Where("username = ?", payload.Username).Count(&count)
	if count > 0 {
		return errors.New("用户名已存在")
	}
	admin := model.Admin{
		PostID:   payload.PostID,
		DeptID:   payload.DeptID,
		Username: payload.Username,
		Password: util.HashPassword(payload.Password),
		Nickname: payload.Nickname,
		Status:   payload.Status,
		Email:    payload.Email,
		Phone:    payload.Phone,
		Note:     payload.Note,
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&admin).Error; err != nil {
			return err
		}
		return tx.Create(&model.AdminRole{AdminID: admin.ID, RoleID: payload.RoleID}).Error
	})
}

func (s *Service) UpdateAdmin(payload AdminPayload) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var admin model.Admin
		if err := tx.First(&admin, payload.ID).Error; err != nil {
			return err
		}
		admin.PostID = payload.PostID
		admin.DeptID = payload.DeptID
		admin.Username = payload.Username
		admin.Nickname = payload.Nickname
		admin.Status = payload.Status
		admin.Email = payload.Email
		admin.Phone = payload.Phone
		admin.Note = payload.Note
		if payload.Password != "" {
			admin.Password = util.HashPassword(payload.Password)
		}
		if err := tx.Save(&admin).Error; err != nil {
			return err
		}
		if err := tx.Where("admin_id = ?", admin.ID).Delete(&model.AdminRole{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.AdminRole{AdminID: admin.ID, RoleID: payload.RoleID}).Error
	})
}

func (s *Service) DeleteAdmin(id uint) error {
	var admin model.Admin
	if err := s.db.First(&admin, id).Error; err != nil {
		return err
	}
	if admin.Username == "admin" {
		return errors.New("默认管理员不允许删除")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.Admin{}, id).Error; err != nil {
			return err
		}
		return tx.Where("admin_id = ?", id).Delete(&model.AdminRole{}).Error
	})
}

func (s *Service) UpdateAdminStatus(payload AdminStatusPayload) error {
	return s.db.Model(&model.Admin{}).Where("id = ?", payload.ID).Update("status", payload.Status).Error
}

func (s *Service) ResetAdminPassword(id uint, password string) error {
	return s.db.Model(&model.Admin{}).Where("id = ?", id).Update("password", util.HashPassword(password)).Error
}

func (s *Service) UpdateProfile(userID uint, payload AdminPayload) error {
	updates := map[string]any{
		"nickname": Trimmed(payload.Nickname),
		"email":    Trimmed(payload.Email),
		"phone":    Trimmed(payload.Phone),
		"note":     Trimmed(payload.Note),
	}
	return s.db.Model(&model.Admin{}).Where("id = ?", userID).Updates(updates).Error
}

func (s *Service) UpdateProfilePassword(userID uint, oldPassword string, newPassword string) error {
	var admin model.Admin
	if err := s.db.First(&admin, userID).Error; err != nil {
		return err
	}
	if !util.CheckPassword(admin.Password, oldPassword) {
		return errors.New("原密码不正确")
	}
	return s.db.Model(&admin).Update("password", util.HashPassword(newPassword)).Error
}

func (s *Service) ListRoles() ([]model.Role, error) {
	var list []model.Role
	return list, s.db.Order("id asc").Find(&list).Error
}

func (s *Service) CreateRole(payload RolePayload) error {
	return s.db.Create(&model.Role{
		RoleName:    payload.RoleName,
		RoleKey:     payload.RoleKey,
		Status:      payload.Status,
		Description: payload.Description,
	}).Error
}

func (s *Service) UpdateRole(payload RolePayload) error {
	return s.db.Model(&model.Role{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"role_name":   payload.RoleName,
		"role_key":    payload.RoleKey,
		"status":      payload.Status,
		"description": payload.Description,
	}).Error
}

func (s *Service) DeleteRole(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.Role{}, id).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		return tx.Where("role_id = ?", id).Delete(&model.AdminRole{}).Error
	})
}

func (s *Service) GetRole(id uint) (*model.Role, error) {
	var role model.Role
	return &role, s.db.First(&role, id).Error
}

func (s *Service) UpdateRoleStatus(payload RoleStatusPayload) error {
	return s.db.Model(&model.Role{}).Where("id = ?", payload.ID).Update("status", payload.Status).Error
}

func (s *Service) AssignRoleMenus(payload RoleMenuPayload) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", payload.ID).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		if len(payload.MenuIDs) == 0 {
			return nil
		}
		items := make([]model.RoleMenu, 0, len(payload.MenuIDs))
		for _, menuID := range payload.MenuIDs {
			items = append(items, model.RoleMenu{RoleID: payload.ID, MenuID: menuID})
		}
		return tx.Create(&items).Error
	})
}

func (s *Service) RoleMenuIDs(roleID uint) ([]uint, error) {
	var ids []uint
	err := s.db.Model(&model.RoleMenu{}).Where("role_id = ?", roleID).Pluck("menu_id", &ids).Error
	return ids, err
}

func (s *Service) ListMenus() ([]model.Menu, error) {
	var list []model.Menu
	return list, s.db.Order("sort asc, id asc").Find(&list).Error
}

func (s *Service) CreateMenu(payload MenuPayload) error {
	return s.db.Create(&model.Menu{
		ParentID:   payload.ParentID,
		MenuName:   payload.MenuName,
		Icon:       payload.Icon,
		Value:      payload.Value,
		MenuType:   payload.MenuType,
		URL:        payload.URL,
		MenuStatus: payload.MenuStatus,
		Sort:       payload.Sort,
	}).Error
}

func (s *Service) UpdateMenu(payload MenuPayload) error {
	return s.db.Model(&model.Menu{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"parent_id":   payload.ParentID,
		"menu_name":   payload.MenuName,
		"icon":        payload.Icon,
		"value":       payload.Value,
		"menu_type":   payload.MenuType,
		"url":         payload.URL,
		"menu_status": payload.MenuStatus,
		"sort":        payload.Sort,
	}).Error
}

func (s *Service) DeleteMenu(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.Menu{}, id).Error; err != nil {
			return err
		}
		if err := tx.Where("parent_id = ?", id).Delete(&model.Menu{}).Error; err != nil {
			return err
		}
		return tx.Where("menu_id = ?", id).Delete(&model.RoleMenu{}).Error
	})
}

func (s *Service) GetMenu(id uint) (*model.Menu, error) {
	var menu model.Menu
	return &menu, s.db.First(&menu, id).Error
}

func (s *Service) ListDepts() ([]model.Dept, error) {
	var list []model.Dept
	return list, s.db.Order("id asc").Find(&list).Error
}

func (s *Service) CreateDept(payload DeptPayload) error {
	return s.db.Create(&model.Dept{
		ParentID:   payload.ParentID,
		DeptType:   payload.DeptType,
		DeptName:   payload.DeptName,
		DeptStatus: payload.DeptStatus,
	}).Error
}

func (s *Service) UpdateDept(payload DeptPayload) error {
	return s.db.Model(&model.Dept{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"parent_id":   payload.ParentID,
		"dept_type":   payload.DeptType,
		"dept_name":   payload.DeptName,
		"dept_status": payload.DeptStatus,
	}).Error
}

func (s *Service) DeleteDept(id uint) error {
	return s.db.Delete(&model.Dept{}, id).Error
}

func (s *Service) GetDept(id uint) (*model.Dept, error) {
	var dept model.Dept
	return &dept, s.db.First(&dept, id).Error
}

func (s *Service) DeptUsers(id uint) ([]model.Admin, error) {
	var list []model.Admin
	return list, s.db.Where("dept_id = ?", id).Find(&list).Error
}

func (s *Service) ListPosts() ([]model.Post, error) {
	var list []model.Post
	return list, s.db.Order("id asc").Find(&list).Error
}

func (s *Service) CreatePost(payload PostPayload) error {
	return s.db.Create(&model.Post{
		PostCode:   payload.PostCode,
		PostName:   payload.PostName,
		PostStatus: payload.PostStatus,
		Remark:     payload.Remark,
	}).Error
}

func (s *Service) UpdatePost(payload PostPayload) error {
	return s.db.Model(&model.Post{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"post_code":   payload.PostCode,
		"post_name":   payload.PostName,
		"post_status": payload.PostStatus,
		"remark":      payload.Remark,
	}).Error
}

func (s *Service) DeletePost(id uint) error {
	return s.db.Delete(&model.Post{}, id).Error
}

func (s *Service) BatchDeletePosts(ids []uint) error {
	return s.db.Delete(&model.Post{}, ids).Error
}

func (s *Service) UpdatePostStatus(payload PostStatusPayload) error {
	return s.db.Model(&model.Post{}).Where("id = ?", payload.ID).Update("post_status", payload.PostStatus).Error
}

func (s *Service) GetPost(id uint) (*model.Post, error) {
	var post model.Post
	return &post, s.db.First(&post, id).Error
}

func (s *Service) ListLoginLogs(pageNum, pageSize int, username string) (map[string]any, error) {
	return s.paginateLogs(&model.LoginLog{}, pageNum, pageSize, "username", username)
}

func (s *Service) DeleteLoginLog(id uint) error {
	return s.db.Delete(&model.LoginLog{}, id).Error
}

func (s *Service) BatchDeleteLoginLogs(ids []uint) error {
	return s.db.Delete(&model.LoginLog{}, ids).Error
}

func (s *Service) CleanLoginLogs() error {
	return s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.LoginLog{}).Error
}

func (s *Service) ListOperationLogs(pageNum, pageSize int, username string, keyword string, riskLevel string, success string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.operationLogQuery(username, keyword, riskLevel, success)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	stats, err := s.operationLogStats(username, keyword, riskLevel, success)
	if err != nil {
		return nil, err
	}
	var list []model.OperationLog
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize, "stats": stats}, nil
}

func (s *Service) operationLogQuery(username string, keyword string, riskLevel string, success string) *gorm.DB {
	query := s.db.Model(&model.OperationLog{})
	if username = strings.TrimSpace(username); username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("description LIKE ? OR url LIKE ? OR request_summary LIKE ? OR ip LIKE ?", like, like, like, like)
	}
	if riskLevel = strings.TrimSpace(riskLevel); riskLevel != "" {
		query = query.Where("risk_level = ?", riskLevel)
	}
	if success = strings.TrimSpace(success); success != "" {
		query = query.Where("success = ?", success == "true" || success == "1")
	}
	return query
}

func (s *Service) operationLogStats(username string, keyword string, riskLevel string, success string) (map[string]any, error) {
	base := s.operationLogQuery(username, keyword, riskLevel, success)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var highRisk int64
	if err := base.Session(&gorm.Session{}).Where("risk_level = ?", "high").Count(&highRisk).Error; err != nil {
		return nil, err
	}
	var failed int64
	if err := base.Session(&gorm.Session{}).Where("success = ?", false).Count(&failed).Error; err != nil {
		return nil, err
	}
	var avgDuration sql.NullFloat64
	if err := base.Session(&gorm.Session{}).Select("AVG(duration_ms)").Scan(&avgDuration).Error; err != nil {
		return nil, err
	}
	avg := 0
	if avgDuration.Valid {
		avg = int(avgDuration.Float64)
	}
	return map[string]any{
		"total":       total,
		"highRisk":    highRisk,
		"failed":      failed,
		"avgDuration": avg,
	}, nil
}

func (s *Service) DeleteOperationLog(id uint) error {
	return s.db.Delete(&model.OperationLog{}, id).Error
}

func (s *Service) BatchDeleteOperationLogs(ids []uint) error {
	return s.db.Delete(&model.OperationLog{}, ids).Error
}

func (s *Service) CleanOperationLogs() error {
	return s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.OperationLog{}).Error
}

// The optional legacyTag argument keeps older callers compatible. Host tags are no
// longer part of host management and the value is intentionally ignored.
func (s *Service) ListAssetHosts(pageNum, pageSize int, keyword string, groupID uint, status, environment string, legacyTag ...string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.AssetHost{}).Preload("Group").Preload("HostGroups").Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").Preload("CloudAccount")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("host_name like ? or alias like ? or private_ip like ? or public_ip like ? or ssh_ip like ?", like, like, like, like, like)
	}
	if groupID > 0 {
		query = query.Joins("LEFT JOIN asset_host_group_rel rel ON rel.host_id = asset_host.id").
			Where("asset_host.group_id = ? OR rel.group_id = ?", groupID, groupID)
	}
	if status != "" {
		query = query.Where("alive_status = ?", status)
	}
	if environment != "" {
		query = query.Where("environment = ?", normalizeEnvCode(environment))
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Distinct("asset_host.id").Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AssetHost
	if err := query.Select("asset_host.*").Distinct().Order("asset_host.id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	ensureHostGroupFallbackList(list)
	s.enrichAssetHostUsageMetrics(list)
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) GetAssetHost(id uint) (*model.AssetHost, error) {
	var item model.AssetHost
	if err := s.db.Preload("Group").Preload("HostGroups").Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").Preload("CloudAccount").First(&item, id).Error; err != nil {
		return &item, err
	}
	ensureHostGroupFallback(&item)
	return &item, nil
}

func (s *Service) CreateAssetHost(payload AssetHostPayload) error {
	host := assetHostFromPayload(payload)
	if host.Environment == "" {
		return errors.New("请选择所属环境")
	}
	if err := validateGatewaySelection(host.ConnectionMode, host.GatewayID); err != nil {
		return err
	}
	if host.SSHPort == 0 {
		host.SSHPort = 22
	}
	if host.Status == 0 {
		host.Status = 1
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&host).Error; err != nil {
			return err
		}
		return syncAssetHostGroups(tx, host.ID, payload.GroupIDs, host.GroupID)
	})
	if err == nil {
		s.recordAssetChange("host", host.ID, host.HostName, "create", "新增主机资产", payload.Operator)
	}
	return err
}

func (s *Service) UpdateAssetHost(payload AssetHostPayload) error {
	updates := assetHostUpdates(payload)
	if normalizeEnvCode(payload.Environment) == "" {
		return errors.New("请选择所属环境")
	}
	if err := validateGatewaySelection(normalizeConnectionMode(payload.ConnectionMode), optionalGatewayID(payload.ConnectionMode, payload.GatewayID)); err != nil {
		return err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AssetHost{}).Where("id = ?", payload.ID).Updates(updates).Error; err != nil {
			return err
		}
		return syncAssetHostGroups(tx, payload.ID, payload.GroupIDs, payload.GroupID)
	})
	if err == nil {
		s.recordAssetChange("host", payload.ID, payload.HostName, "update", "更新主机基础信息", payload.Operator)
	}
	return err
}

func (s *Service) SyncAssetHost(id uint) (*model.AssetHost, error) {
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, id).Error; err != nil {
		return nil, err
	}

	deadline := time.Now().Add(10 * time.Second)
	now := time.Now()
	updates := map[string]any{
		"alive_status":    2,
		"auth_status":     2,
		"last_check_time": &now,
	}

	address := net.JoinHostPort(host.SSHIP, strconv.Itoa(host.SSHPort))
	connectTimeout := remainingTimeout(deadline, 3*time.Second)
	if normalizeConnectionMode(host.ConnectionMode) == "gateway" && host.GatewayID != nil && *host.GatewayID > 0 {
		if connectTimeout > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
			if conn, cleanup, err := s.dialThroughGateway(ctx, *host.GatewayID, "tcp", address); err == nil {
				_ = conn.Close()
				cleanup()
				updates["alive_status"] = 1
			}
			cancel()
		}
	} else if connectTimeout > 0 {
		conn, err := net.DialTimeout("tcp", address, connectTimeout)
		if err == nil {
			_ = conn.Close()
			updates["alive_status"] = 1
		}
	}

	sshTimeout := remainingTimeout(deadline, 5*time.Second)
	if sshTimeout <= 0 {
		_ = s.db.Model(&host).Updates(updates).Error
		return s.GetAssetHost(id)
	}
	client, err := s.newSSHClientWithTimeout(host, sshTimeout)
	if err != nil {
		if time.Until(deadline) <= 0 {
			updates["alive_status"] = 2
		}
		_ = s.db.Model(&host).Updates(updates).Error
		return s.GetAssetHost(id)
	}
	defer client.Close()
	updates["auth_status"] = 1
	updates["status"] = 1

	info, timedOut := collectHostInfo(client, deadline)
	if timedOut || time.Until(deadline) <= 0 {
		updates["alive_status"] = 2
	}
	if len(info) > 0 {
		for key, value := range info {
			if strings.TrimSpace(value) != "" {
				updates[key] = value
			}
		}
	}

	if err := s.db.Model(&host).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.recordAssetChange("host", host.ID, host.HostName, "sync", "同步主机配置与连接状态", "system")
	return s.GetAssetHost(id)
}

func (s *Service) ImportAssetHosts(groupID uint, rows []AssetHostImportRow) (map[string]any, error) {
	var group model.AssetHostGroup
	if err := s.db.First(&group, groupID).Error; err != nil {
		return nil, errors.New("host group not found")
	}

	successCount := 0
	failed := make([]string, 0)

	for _, row := range rows {
		hostName := Trimmed(row.HostName)
		sshIP := Trimmed(row.SSHIP)
		sshUser := Trimmed(row.SSHUser)
		credentialName := Trimmed(row.CredentialName)
		environment := normalizeEnvCode(row.Environment)
		connectionMode := normalizeConnectionMode(row.ConnectionMode)
		if hostName == "" || sshIP == "" || sshUser == "" || credentialName == "" || environment == "" {
			failed = append(failed, hostName+"(missing fields)")
			continue
		}

		var credential model.AssetCredential
		if err := s.db.Where("name = ? AND status = ?", credentialName, 1).First(&credential).Error; err != nil {
			failed = append(failed, hostName+"(credential not found)")
			continue
		}
		var gatewayID *uint
		if connectionMode == "gateway" {
			gatewayName := Trimmed(row.GatewayName)
			if gatewayName == "" {
				failed = append(failed, hostName+"(gateway is required)")
				continue
			}
			var gateway model.AssetGateway
			if err := s.db.Where("name = ? AND status = ?", gatewayName, 1).First(&gateway).Error; err != nil {
				failed = append(failed, hostName+"(gateway not found)")
				continue
			}
			gatewayID = &gateway.ID
		}

		var existing model.AssetHost
		if err := s.db.Where("ssh_ip = ?", sshIP).First(&existing).Error; err == nil {
			failed = append(failed, hostName+"(ssh ip exists)")
			continue
		}

		host := model.AssetHost{
			HostName:       hostName,
			GroupID:        groupID,
			CredentialID:   optionalUint(credential.ID),
			ConnectionMode: connectionMode,
			GatewayID:      gatewayID,
			SSHIP:          sshIP,
			SSHUser:        sshUser,
			SSHPort:        defaultSSHPort(row.SSHPort),
			PrivateIP:      Trimmed(row.PrivateIP),
			PublicIP:       Trimmed(row.PublicIP),
			Environment:    environment,
			Status:         1,
			AuthStatus:     2,
			AliveStatus:    2,
			Description:    Trimmed(row.Description),
			Provider:       firstNonEmpty(Trimmed(row.Provider), "自建"),
			Region:         Trimmed(row.Region),
		}
		if err := s.db.Create(&host).Error; err != nil {
			failed = append(failed, hostName+"(create failed)")
			continue
		}
		_ = syncAssetHostGroups(s.db, host.ID, []uint{groupID}, groupID)
		successCount++
	}

	return map[string]any{
		"success":     successCount,
		"fail":        len(failed),
		"total":       len(rows),
		"failedHosts": failed,
	}, nil
}

func (s *Service) SyncAssetHostsFromCloud(payload AssetCloudSyncPayload) (map[string]any, error) {
	provider := strings.ToLower(Trimmed(payload.Provider))
	if payload.GroupID == 0 {
		return nil, errors.New("groupId is required")
	}
	credentialID := optionalUint(payload.CredentialID)
	if credentialID == nil {
		return nil, errors.New("请选择认证凭据")
	}
	var credential model.AssetCredential
	if err := s.db.Where("id = ? AND status = ?", *credentialID, 1).First(&credential).Error; err != nil {
		return nil, errors.New("所选认证凭据不存在或已停用")
	}
	environment := normalizeEnvCode(payload.Environment)
	if environment == "" {
		return nil, errors.New("请选择所属环境")
	}
	connectionMode := normalizeConnectionMode(payload.ConnectionMode)
	gatewayID := optionalGatewayID(connectionMode, payload.GatewayID)
	if err := validateGatewaySelection(connectionMode, gatewayID); err != nil {
		return nil, err
	}
	if gatewayID != nil {
		var gateway model.AssetGateway
		if err := s.db.Where("id = ? AND status = ?", *gatewayID, 1).First(&gateway).Error; err != nil {
			return nil, errors.New("所选访问网关不存在或已停用")
		}
	}
	if !payload.UseExistingAccount {
		return nil, errors.New("cloud sync requires a configured cloud account with at least one region")
	}

	accessKey := Trimmed(payload.AccessKey)
	secretKey := Trimmed(payload.SecretKey)
	regions := normalizeCloudRegions(payload.Regions, payload.Region)
	region := strings.Join(regions, ",")
	var accountID *uint

	if payload.UseExistingAccount {
		var account model.AssetCloudAccount
		if err := s.db.First(&account, payload.CloudAccountID).Error; err != nil {
			return nil, errors.New("cloud account not found")
		}
		provider = strings.ToLower(Trimmed(account.Provider))
		accessKey = Trimmed(account.AccessKey)
		secretKey = Trimmed(account.SecretKey)
		if len(regions) == 0 {
			regions = normalizeCloudRegions(account.Regions, account.Region)
			region = strings.Join(regions, ",")
		}
		accountID = &account.ID
	}

	instances, err := fetchCloudInstances(provider, accessKey, secretKey, region)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, errors.New("no instances found")
	}

	added := 0
	updated := 0
	skipped := 0
	addedHosts := make([]string, 0)
	updatedHosts := make([]string, 0)
	skippedHosts := make([]string, 0)
	regionCounts := make(map[string]int)
	for _, item := range instances {
		regionCounts[firstNonEmpty(item.Region, "unknown")]++
		// Cloud hosts should be managed on their VPC address first. A public address
		// remains inventory metadata and is only a fallback when no private address exists.
		sshIP := firstNonEmpty(item.PrivateIP, item.PublicIP)
		if sshIP == "" {
			skipped++
			skippedHosts = append(skippedHosts, firstNonEmpty(item.HostName, item.InstanceID, "unknown")+"：未返回公网或私网 IP")
			continue
		}

		var host model.AssetHost
		lookup := s.db.Model(&model.AssetHost{})
		if accountID != nil && item.InstanceID != "" {
			// A manually managed host may use the same IP. Only a record from the
			// same cloud account with the same cloud instance ID is an idempotent match.
			lookup = lookup.Where("cloud_account_id = ? AND instance_id = ?", *accountID, item.InstanceID)
		} else if item.InstanceID != "" {
			lookup = lookup.Where("provider = ? AND instance_id = ?", provider, item.InstanceID)
		} else if accountID != nil {
			lookup = lookup.Where("cloud_account_id = ? AND ssh_ip = ?", *accountID, sshIP)
		} else {
			lookup = lookup.Where("provider = ? AND ssh_ip = ?", provider, sshIP)
		}
		err := lookup.First(&host).Error
		switch {
		case err == nil:
			// Discovery must never overwrite an existing host. Cloud credentials,
			// routing, environment and manually maintained host metadata stay intact.
			skipped++
			skippedHosts = append(skippedHosts, firstNonEmpty(item.HostName, item.InstanceID, sshIP)+"：主机已存在，未覆盖")
		case errors.Is(err, gorm.ErrRecordNotFound):
			newHost := model.AssetHost{
				HostName:       firstNonEmpty(item.HostName, item.InstanceID, sshIP),
				GroupID:        payload.GroupID,
				PrivateIP:      item.PrivateIP,
				PublicIP:       item.PublicIP,
				SSHIP:          sshIP,
				SSHUser:        firstNonEmpty(item.SSHUser, "root"),
				SSHPort:        defaultSSHPort(item.SSHPort),
				OS:             item.OS,
				CPU:            item.CPU,
				Memory:         item.Memory,
				Disk:           item.Disk,
				Provider:       provider,
				Region:         item.Region,
				InstanceID:     item.InstanceID,
				CloudAccountID: accountID,
				CredentialID:   credentialID,
				ConnectionMode: connectionMode,
				GatewayID:      gatewayID,
				Environment:    environment,
				Status:         1,
				AliveStatus:    2,
				AuthStatus:     2,
			}
			if createErr := s.db.Create(&newHost).Error; createErr == nil {
				_ = syncAssetHostGroups(s.db, newHost.ID, []uint{payload.GroupID}, payload.GroupID)
				added++
				addedHosts = append(addedHosts, newHost.HostName)
			} else {
				skipped++
				skippedHosts = append(skippedHosts, newHost.HostName+"：新增失败："+createErr.Error())
			}
		default:
			skipped++
			skippedHosts = append(skippedHosts, firstNonEmpty(item.HostName, item.InstanceID, sshIP)+"：查询已有主机失败")
		}
	}

	return map[string]any{
		"provider":     provider,
		"total":        len(instances),
		"added":        added,
		"addedHosts":   addedHosts,
		"updated":      updated,
		"updatedHosts": updatedHosts,
		"skipped":      skipped,
		"skippedHosts": skippedHosts,
		"regionCounts": regionCounts,
	}, nil
}

func (s *Service) OpenAssetTerminal(id uint, rows, cols int) (*AssetTerminalSession, error) {
	var host model.AssetHost
	if err := s.db.Preload("Credential").Preload("Gateway").Preload("Gateway.Credential").First(&host, id).Error; err != nil {
		return nil, err
	}
	if rows <= 0 {
		rows = 30
	}
	if cols <= 0 {
		cols = 120
	}

	client, err := s.newSSHClient(host)
	if err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return nil, err
	}

	return &AssetTerminalSession{
		Host:    host,
		Client:  client,
		Session: session,
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
	}, nil
}

func (s *Service) DeleteAssetHost(id uint) error {
	var host model.AssetHost
	_ = s.db.First(&host, id).Error
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("host_id = ?", id).Delete(&model.AssetHostGroupRelation{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.AssetHost{}, id).Error
	})
	if err == nil {
		s.recordAssetChange("host", id, host.HostName, "delete", "删除主机资产", "system")
	}
	return err
}

func (s *Service) BatchDeleteAssetHosts(ids []uint) error {
	if len(ids) == 0 {
		return errors.New("no host selected")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("host_id IN ?", ids).Delete(&model.AssetHostGroupRelation{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&model.AssetHost{}).Error
	})
}

func (s *Service) RemoveAssetHostsFromGroup(groupID uint, hostIDs []uint) error {
	if groupID == 0 {
		return errors.New("host group is required")
	}
	if len(hostIDs) == 0 {
		return errors.New("no host selected")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ? AND host_id IN ?", groupID, hostIDs).Delete(&model.AssetHostGroupRelation{}).Error; err != nil {
			return err
		}
		for _, hostID := range hostIDs {
			var host model.AssetHost
			if err := tx.Select("id", "group_id").First(&host, hostID).Error; err != nil {
				return err
			}
			if host.GroupID != groupID {
				continue
			}
			var nextGroupID uint
			if err := tx.Model(&model.AssetHostGroupRelation{}).
				Where("host_id = ?", hostID).
				Order("group_id asc").
				Limit(1).
				Pluck("group_id", &nextGroupID).Error; err != nil {
				return err
			}
			nextGroupValue := any(nil)
			if nextGroupID > 0 {
				nextGroupValue = nextGroupID
			}
			if err := tx.Model(&model.AssetHost{}).Where("id = ?", hostID).Update("group_id", nextGroupValue).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) BatchSyncAssetHosts(ids []uint) (map[string]any, error) {
	if len(ids) == 0 {
		return nil, errors.New("no host selected")
	}
	type result struct {
		id  uint
		err error
	}
	workerCount := len(ids)
	if workerCount > 4 {
		workerCount = 4
	}
	jobs := make(chan uint)
	results := make(chan result, len(ids))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				_, err := s.SyncAssetHost(id)
				results <- result{id: id, err: err}
			}
		}()
	}
	go func() {
		for _, id := range ids {
			jobs <- id
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	successCount := 0
	failed := make([]uint, 0)
	for item := range results {
		if item.err != nil {
			failed = append(failed, item.id)
			continue
		}
		successCount++
	}
	return map[string]any{
		"success":   successCount,
		"fail":      len(failed),
		"failedIds": failed,
	}, nil
}

func (s *Service) BatchReplaceAssetHostCredential(ids []uint, credentialID uint) error {
	if len(ids) == 0 {
		return errors.New("no host selected")
	}
	if credentialID == 0 {
		return errors.New("credentialId is required")
	}
	var credential model.AssetCredential
	if err := s.db.First(&credential, credentialID).Error; err != nil {
		return errors.New("credential not found")
	}
	return s.db.Model(&model.AssetHost{}).Where("id IN ?", ids).Updates(map[string]any{
		"credential_id": credentialID,
		"auth_status":   2,
	}).Error
}

func (s *Service) ListAssetHostGroups(keyword string) (*AssetHostGroupListResult, error) {
	var groups []model.AssetHostGroup
	query := s.db.Model(&model.AssetHostGroup{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name like ? or code like ?", like, like)
	}
	if err := query.Order("sort asc, id asc").Find(&groups).Error; err != nil {
		return nil, err
	}
	countMap, err := s.assetHostGroupHostCountMap()
	if err != nil {
		return nil, err
	}
	list := make([]AssetHostGroupNode, 0, len(groups))
	for _, item := range groups {
		list = append(list, s.newAssetHostGroupNode(item, countMap[item.ID]))
	}
	return &AssetHostGroupListResult{
		List:  list,
		Tree:  buildAssetHostGroupTree(list),
		Total: len(list),
	}, nil
}

func (s *Service) GetAssetHostGroup(id uint) (*AssetHostGroupNode, error) {
	var item model.AssetHostGroup
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	countMap, err := s.assetHostGroupHostCountMap()
	if err != nil {
		return nil, err
	}
	result := s.newAssetHostGroupNode(item, countMap[item.ID])
	return &result, nil
}

func (s *Service) CreateAssetHostGroup(payload AssetHostGroupPayload) error {
	name := Trimmed(payload.Name)
	if name == "" {
		return errors.New("主机组名称不能为空")
	}
	if err := s.validateAssetHostGroupParent(0, payload.ParentID); err != nil {
		return err
	}
	item := model.AssetHostGroup{
		ParentID:    payload.ParentID,
		Name:        name,
		Code:        Trimmed(payload.Code),
		Sort:        payload.Sort,
		Status:      payload.Status,
		Description: Trimmed(payload.Description),
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateAssetHostGroup(payload AssetHostGroupPayload) error {
	name := Trimmed(payload.Name)
	if payload.ID == 0 {
		return errors.New("主机组不存在")
	}
	if name == "" {
		return errors.New("主机组名称不能为空")
	}
	if err := s.validateAssetHostGroupParent(payload.ID, payload.ParentID); err != nil {
		return err
	}
	return s.db.Model(&model.AssetHostGroup{}).Where("id = ?", payload.ID).Updates(map[string]any{
		"parent_id":   payload.ParentID,
		"name":        name,
		"code":        Trimmed(payload.Code),
		"sort":        payload.Sort,
		"status":      payload.Status,
		"description": Trimmed(payload.Description),
	}).Error
}

func (s *Service) DeleteAssetHostGroup(id uint) error {
	if id == 0 {
		return errors.New("主机组不存在")
	}
	hostCount, err := s.assetHostGroupHostCount(id)
	if err != nil {
		return err
	}
	if hostCount > 0 {
		return errors.New("当前主机组下仍有关联主机，不能删除")
	}
	var childCount int64
	if err := s.db.Model(&model.AssetHostGroup{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		return err
	}
	if childCount > 0 {
		return errors.New("当前主机组下仍有子分组，不能删除")
	}
	return s.db.Delete(&model.AssetHostGroup{}, id).Error
}

func (s *Service) validateAssetHostGroupParent(id uint, parentID uint) error {
	if parentID == 0 {
		return nil
	}
	if id != 0 && id == parentID {
		return errors.New("上级主机组不能选择自己")
	}
	var parent model.AssetHostGroup
	if err := s.db.First(&parent, parentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("上级主机组不存在")
		}
		return err
	}
	if id == 0 {
		return nil
	}
	var groups []model.AssetHostGroup
	if err := s.db.Select("id", "parent_id").Find(&groups).Error; err != nil {
		return err
	}
	parentMap := make(map[uint]uint, len(groups))
	for _, item := range groups {
		parentMap[item.ID] = item.ParentID
	}
	for current := parentID; current != 0; {
		if current == id {
			return errors.New("上级主机组不能选择当前分组或其子分组")
		}
		next, ok := parentMap[current]
		if !ok || next == current {
			break
		}
		current = next
	}
	return nil
}

func (s *Service) assetHostGroupHostCount(id uint) (int64, error) {
	countMap, err := s.assetHostGroupHostCountMap()
	if err != nil {
		return 0, err
	}
	return countMap[id], nil
}

func (s *Service) assetHostGroupHostCountMap() (map[uint]int64, error) {
	type countRow struct {
		GroupID uint  `gorm:"column:group_id"`
		Count   int64 `gorm:"column:count"`
	}

	countMap := map[uint]int64{}

	var relRows []countRow
	if err := s.db.Model(&model.AssetHostGroupRelation{}).
		Select("group_id, count(distinct host_id) as count").
		Group("group_id").
		Scan(&relRows).Error; err != nil {
		return nil, err
	}
	for _, item := range relRows {
		countMap[item.GroupID] += item.Count
	}

	var fallbackRows []countRow
	if err := s.db.Model(&model.AssetHost{}).
		Select("asset_host.group_id as group_id, count(distinct asset_host.id) as count").
		Joins("LEFT JOIN asset_host_group_rel rel ON rel.host_id = asset_host.id AND rel.group_id = asset_host.group_id").
		Where("asset_host.group_id <> 0 AND rel.host_id IS NULL").
		Group("asset_host.group_id").
		Scan(&fallbackRows).Error; err != nil {
		return nil, err
	}
	for _, item := range fallbackRows {
		countMap[item.GroupID] += item.Count
	}

	return countMap, nil
}

func (s *Service) newAssetHostGroupNode(item model.AssetHostGroup, hostCount int64) AssetHostGroupNode {
	return AssetHostGroupNode{
		ID:          item.ID,
		ParentID:    item.ParentID,
		Name:        item.Name,
		Code:        item.Code,
		Sort:        item.Sort,
		Status:      item.Status,
		Description: item.Description,
		HostCount:   hostCount,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func buildAssetHostGroupTree(list []AssetHostGroupNode) []*AssetHostGroupNode {
	nodes := make(map[uint]*AssetHostGroupNode, len(list))
	roots := make([]*AssetHostGroupNode, 0)
	for _, item := range list {
		node := item
		node.Children = nil
		nodes[node.ID] = &node
	}
	for _, item := range list {
		node := nodes[item.ID]
		if node.ParentID != 0 {
			if parent, ok := nodes[node.ParentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	sortAssetHostGroupNodes(roots)
	return roots
}

func sortAssetHostGroupNodes(nodes []*AssetHostGroupNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Sort == nodes[j].Sort {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].Sort < nodes[j].Sort
	})
	for _, node := range nodes {
		if len(node.Children) > 0 {
			sortAssetHostGroupNodes(node.Children)
		}
	}
}

func (s *Service) ListAssetCredentials(pageNum, pageSize int, keyword string, authType string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.AssetCredential{})
	if keyword != "" {
		query = query.Where("name like ? or username like ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if authType != "" {
		query = query.Where("auth_type = ?", authType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AssetCredential
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		maskCredential(&list[i])
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListAssetCredentialOptions() ([]model.AssetCredential, error) {
	var list []model.AssetCredential
	err := s.db.Where("status = ?", 1).Order("id desc").Find(&list).Error
	for i := range list {
		maskCredential(&list[i])
	}
	return list, err
}

func (s *Service) GetAssetCredential(id uint) (*model.AssetCredential, error) {
	var item model.AssetCredential
	return &item, s.db.First(&item, id).Error
}

func (s *Service) CreateAssetCredential(payload AssetCredentialPayload) error {
	item := model.AssetCredential{
		Name:        Trimmed(payload.Name),
		AuthType:    normalizedAuthType(payload.AuthType),
		Username:    Trimmed(payload.Username),
		Password:    payload.Password,
		PrivateKey:  payload.PrivateKey,
		Passphrase:  payload.Passphrase,
		Status:      payload.Status,
		Description: Trimmed(payload.Description),
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateAssetCredential(payload AssetCredentialPayload) error {
	updates := map[string]any{
		"name":        Trimmed(payload.Name),
		"auth_type":   normalizedAuthType(payload.AuthType),
		"username":    Trimmed(payload.Username),
		"status":      payload.Status,
		"description": Trimmed(payload.Description),
	}
	if payload.Password != "" {
		updates["password"] = payload.Password
	}
	if payload.PrivateKey != "" {
		updates["private_key"] = payload.PrivateKey
	}
	if payload.Passphrase != "" {
		updates["passphrase"] = payload.Passphrase
	}
	return s.db.Model(&model.AssetCredential{}).Where("id = ?", payload.ID).Updates(updates).Error
}

func (s *Service) DeleteAssetCredential(id uint) error {
	var count int64
	s.db.Model(&model.AssetHost{}).Where("credential_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("凭据已被主机关联，不能删除")
	}
	return s.db.Delete(&model.AssetCredential{}, id).Error
}

func (s *Service) ListAssetCloudAccounts(pageNum, pageSize int, keyword string, provider string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.AssetCloudAccount{})
	if keyword != "" {
		query = query.Where("name like ? or access_key like ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.AssetCloudAccount
	if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		hydrateCloudAccountRegions(&list[i])
		list[i].SecretKey = maskedSecret(list[i].SecretKey)
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListAssetCloudAccountOptions() ([]model.AssetCloudAccount, error) {
	var list []model.AssetCloudAccount
	err := s.db.Where("status = ?", 1).Order("id desc").Find(&list).Error
	for i := range list {
		hydrateCloudAccountRegions(&list[i])
		list[i].SecretKey = maskedSecret(list[i].SecretKey)
	}
	return list, err
}

func (s *Service) GetAssetCloudAccount(id uint) (*model.AssetCloudAccount, error) {
	var item model.AssetCloudAccount
	if err := s.db.First(&item, id).Error; err != nil {
		return &item, err
	}
	hydrateCloudAccountRegions(&item)
	return &item, nil
}

func normalizeCloudRegions(regions []string, legacy string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(regions)+1)
	appendRegion := func(value string) {
		for _, region := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			region = strings.TrimSpace(region)
			if region == "" {
				continue
			}
			if _, exists := seen[region]; exists {
				continue
			}
			seen[region] = struct{}{}
			result = append(result, region)
		}
	}
	for _, region := range regions {
		appendRegion(region)
	}
	appendRegion(legacy)
	return result
}

func cloudRegionsJSON(regions []string) string {
	encoded, err := json.Marshal(regions)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func hydrateCloudAccountRegions(account *model.AssetCloudAccount) {
	account.Regions = normalizeCloudRegions(account.Regions, account.Region)
	if account.Region == "" {
		account.Region = strings.Join(account.Regions, ",")
	}
}

func (s *Service) CreateAssetCloudAccount(payload AssetCloudAccountPayload) error {
	regions := normalizeCloudRegions(payload.Regions, payload.Region)
	if len(regions) == 0 {
		return errors.New("at least one sync region is required")
	}
	item := model.AssetCloudAccount{
		Name:        Trimmed(payload.Name),
		Provider:    Trimmed(payload.Provider),
		AccessKey:   Trimmed(payload.AccessKey),
		SecretKey:   payload.SecretKey,
		Regions:     regions,
		Region:      strings.Join(regions, ","),
		Status:      payload.Status,
		Description: Trimmed(payload.Description),
	}
	if item.Status == 0 {
		item.Status = 1
	}
	return s.db.Create(&item).Error
}

func (s *Service) UpdateAssetCloudAccount(payload AssetCloudAccountPayload) error {
	regions := normalizeCloudRegions(payload.Regions, payload.Region)
	if len(regions) == 0 {
		return errors.New("at least one sync region is required")
	}
	updates := map[string]any{
		"name":        Trimmed(payload.Name),
		"provider":    Trimmed(payload.Provider),
		"access_key":  Trimmed(payload.AccessKey),
		"regions":     cloudRegionsJSON(regions),
		"region":      strings.Join(regions, ","),
		"status":      payload.Status,
		"description": Trimmed(payload.Description),
	}
	if payload.SecretKey != "" {
		updates["secret_key"] = payload.SecretKey
	}
	return s.db.Model(&model.AssetCloudAccount{}).Where("id = ?", payload.ID).Updates(updates).Error
}

func (s *Service) DeleteAssetCloudAccount(id uint) error {
	var count int64
	s.db.Model(&model.AssetHost{}).Where("cloud_account_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("云账号已被主机关联，不能删除")
	}
	return s.db.Delete(&model.AssetCloudAccount{}, id).Error
}

func (s *Service) GetAssetOverview() (map[string]any, error) {
	var (
		hostTotal           int64
		hostOnline          int64
		hostOffline         int64
		hostAuthFailed      int64
		groupTotal          int64
		credentialTotal     int64
		credentialEnabled   int64
		cloudAccountTotal   int64
		cloudAccountEnabled int64
		databaseTotal       int64
		databaseHealthy     int64
		k8sClusterTotal     int64
		k8sClusterOnline    int64
		k8sNodeTotal        int64
		incompleteHosts     int64
		incompleteDatabases int64
		incompleteClusters  int64
	)

	if err := s.db.Model(&model.AssetHost{}).Count(&hostTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetHost{}).Where("alive_status = ?", 1).Count(&hostOnline).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetHost{}).Where("alive_status = ?", 2).Count(&hostOffline).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetHost{}).Where("auth_status = ?", 2).Count(&hostAuthFailed).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetHostGroup{}).Count(&groupTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetCredential{}).Count(&credentialTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetCredential{}).Where("status = ?", 1).Count(&credentialEnabled).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetCloudAccount{}).Count(&cloudAccountTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetCloudAccount{}).Where("status = ?", 1).Count(&cloudAccountEnabled).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetDatabase{}).Count(&databaseTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetDatabase{}).Where("connect_status = ?", 1).Count(&databaseHealthy).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.K8sCluster{}).Count(&k8sClusterTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.K8sCluster{}).Where("status = ?", "running").Count(&k8sClusterOnline).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.K8sCluster{}).Select("COALESCE(SUM(node_count), 0)").Scan(&k8sNodeTotal).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetHost{}).Where("TRIM(COALESCE(environment, '')) = ''").Count(&incompleteHosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.AssetDatabase{}).Where("TRIM(COALESCE(env, '')) = ''").Count(&incompleteDatabases).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&model.K8sCluster{}).Where("TRIM(COALESCE(env, '')) = ''").Count(&incompleteClusters).Error; err != nil {
		return nil, err
	}

	type distributionRow struct {
		Name  string `json:"name" gorm:"column:name"`
		Count int64  `json:"count" gorm:"column:count"`
	}

	var providerRows []distributionRow
	if err := s.db.Model(&model.AssetHost{}).
		Select("COALESCE(NULLIF(provider, ''), '自建') as name, COUNT(*) as count").
		Group("COALESCE(NULLIF(provider, ''), '自建')").
		Order("count DESC").
		Limit(8).
		Scan(&providerRows).Error; err != nil {
		return nil, err
	}

	var environmentRows []distributionRow
	if err := s.db.Model(&model.AssetHost{}).
		Where("NULLIF(environment, '') IS NOT NULL").
		Select("environment as name, COUNT(*) as count").
		Group("environment").
		Order("count DESC").
		Limit(8).
		Scan(&environmentRows).Error; err != nil {
		return nil, err
	}

	var groups []model.AssetHostGroup
	if err := s.db.Order("sort ASC, updated_at DESC").Find(&groups).Error; err != nil {
		return nil, err
	}
	countMap, err := s.assetHostGroupHostCountMap()
	if err != nil {
		return nil, err
	}

	groupItems := make([]map[string]any, 0, len(groups))
	for _, item := range groups {
		groupItems = append(groupItems, map[string]any{
			"id":        item.ID,
			"name":      item.Name,
			"code":      item.Code,
			"status":    item.Status,
			"hostCount": countMap[item.ID],
			"updatedAt": item.UpdatedAt,
		})
	}
	sort.Slice(groupItems, func(i, j int) bool {
		return groupItems[i]["hostCount"].(int64) > groupItems[j]["hostCount"].(int64)
	})
	if len(groupItems) > 6 {
		groupItems = groupItems[:6]
	}

	var recentHosts []model.AssetHost
	if err := s.db.Preload("Group").Preload("HostGroups").Preload("Credential").Preload("CloudAccount").
		Order("updated_at DESC").
		Limit(6).
		Find(&recentHosts).Error; err != nil {
		return nil, err
	}
	ensureHostGroupFallbackList(recentHosts)

	hostItems := make([]map[string]any, 0, len(recentHosts))
	for _, item := range recentHosts {
		groupNames := make([]string, 0, len(item.HostGroups))
		for _, group := range item.HostGroups {
			groupNames = append(groupNames, group.Name)
		}
		hostItems = append(hostItems, map[string]any{
			"id":          item.ID,
			"hostName":    item.HostName,
			"sshIp":       item.SSHIP,
			"publicIp":    item.PublicIP,
			"privateIp":   item.PrivateIP,
			"provider":    item.Provider,
			"environment": item.Environment,
			"aliveStatus": item.AliveStatus,
			"authStatus":  item.AuthStatus,
			"groupNames":  groupNames,
			"updatedAt":   item.UpdatedAt,
		})
	}

	var recentDatabases []model.AssetDatabase
	if err := s.db.Order("updated_at DESC").Limit(6).Find(&recentDatabases).Error; err != nil {
		return nil, err
	}
	databaseItems := make([]map[string]any, 0, len(recentDatabases))
	for _, item := range recentDatabases {
		databaseItems = append(databaseItems, map[string]any{
			"id":            item.ID,
			"name":          item.Name,
			"dbType":        item.DBType,
			"host":          item.Host,
			"port":          item.Port,
			"dbName":        item.DBName,
			"version":       item.Version,
			"status":        item.Status,
			"connectStatus": item.ConnectStatus,
			"updatedAt":     item.UpdatedAt,
		})
	}

	var recentClusters []model.K8sCluster
	if err := s.db.Order("updated_at DESC").Limit(6).Find(&recentClusters).Error; err != nil {
		return nil, err
	}
	clusterItems := make([]map[string]any, 0, len(recentClusters))
	for _, item := range recentClusters {
		clusterItems = append(clusterItems, map[string]any{
			"id":        item.ID,
			"name":      item.Name,
			"status":    item.Status,
			"apiServer": item.APIServer,
			"version":   item.Version,
			"nodeCount": item.NodeCount,
			"updatedAt": item.UpdatedAt,
		})
	}

	return map[string]any{
		"summary": map[string]any{
			"hostTotal":           hostTotal,
			"hostOnline":          hostOnline,
			"hostOffline":         hostOffline,
			"groupTotal":          groupTotal,
			"credentialTotal":     credentialTotal,
			"credentialEnabled":   credentialEnabled,
			"cloudAccountTotal":   cloudAccountTotal,
			"cloudAccountEnabled": cloudAccountEnabled,
			"databaseTotal":       databaseTotal,
			"databaseHealthy":     databaseHealthy,
			"k8sClusterTotal":     k8sClusterTotal,
			"k8sClusterOnline":    k8sClusterOnline,
			"k8sNodeTotal":        k8sNodeTotal,
		},
		"health": map[string]any{
			"offlineHosts":        hostOffline,
			"authFailedHosts":     hostAuthFailed,
			"healthyDatabases":    databaseHealthy,
			"abnormalDatabases":   databaseTotal - databaseHealthy,
			"abnormalClusters":    k8sClusterTotal - k8sClusterOnline,
			"incompleteAssets":    incompleteHosts + incompleteDatabases + incompleteClusters,
			"incompleteHosts":     incompleteHosts,
			"incompleteDatabases": incompleteDatabases,
			"incompleteClusters":  incompleteClusters,
		},
		"distributions": map[string]any{
			"providers":    providerRows,
			"environments": environmentRows,
		},
		"topGroups":       groupItems,
		"recentHosts":     hostItems,
		"recentDatabases": databaseItems,
		"recentClusters":  clusterItems,
	}, nil
}

func (s *Service) paginateLogs(entity any, pageNum, pageSize int, field string, keyword string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(entity)
	if keyword != "" {
		query = query.Where(field+" like ?", "%"+keyword+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	switch entity.(type) {
	case *model.LoginLog:
		var list []model.LoginLog
		if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
			return nil, err
		}
		return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
	case *model.OperationLog:
		var list []model.OperationLog
		if err := query.Order("id desc").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
			return nil, err
		}
		return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
	default:
		return nil, errors.New("unsupported entity")
	}
}

func Trimmed(s string) string {
	return strings.TrimSpace(s)
}

func normalizeBrowser(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "edg/"):
		return "Edge"
	case strings.Contains(ua, "chrome/"):
		return "Chrome"
	case strings.Contains(ua, "firefox/"):
		return "Firefox"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome/"):
		return "Safari"
	case strings.Contains(ua, "micromessenger"):
		return "WeChat"
	default:
		return shortenText(strings.TrimSpace(userAgent), 120)
	}
}

func normalizeOS(userAgent string, fallback string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "windows"):
		return "Windows"
	case strings.Contains(ua, "mac os x"), strings.Contains(ua, "macintosh"):
		return "macOS"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "ios"):
		return "iOS"
	case strings.Contains(ua, "linux"):
		return "Linux"
	default:
		return shortenText(strings.TrimSpace(fallback), 120)
	}
}

func shortenText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func assetHostFromPayload(payload AssetHostPayload) model.AssetHost {
	groupIDs := normalizeGroupIDs(payload.GroupIDs, payload.GroupID)
	return model.AssetHost{
		HostName:       Trimmed(payload.HostName),
		Alias:          Trimmed(payload.Alias),
		GroupID:        firstGroupID(groupIDs),
		CredentialID:   optionalUint(payload.CredentialID),
		ConnectionMode: normalizeConnectionMode(payload.ConnectionMode),
		GatewayID:      optionalGatewayID(payload.ConnectionMode, payload.GatewayID),
		CloudAccountID: optionalUint(payload.CloudAccountID),
		PrivateIP:      Trimmed(payload.PrivateIP),
		PublicIP:       Trimmed(payload.PublicIP),
		SSHUser:        Trimmed(payload.SSHUser),
		SSHIP:          Trimmed(payload.SSHIP),
		SSHPort:        payload.SSHPort,
		OS:             Trimmed(payload.OS),
		Arch:           Trimmed(payload.Arch),
		CPU:            Trimmed(payload.CPU),
		Memory:         Trimmed(payload.Memory),
		Disk:           Trimmed(payload.Disk),
		Environment:    normalizeEnvCode(payload.Environment),
		Provider:       Trimmed(payload.Provider),
		Region:         Trimmed(payload.Region),
		Status:         payload.Status,
		Description:    Trimmed(payload.Description),
	}
}

func optionalUint(value uint) *uint {
	if value == 0 {
		return nil
	}
	return &value
}

func assetHostUpdates(payload AssetHostPayload) map[string]any {
	host := assetHostFromPayload(payload)
	if host.SSHPort == 0 {
		host.SSHPort = 22
	}
	if host.Status == 0 {
		host.Status = 1
	}
	return map[string]any{
		"host_name":        host.HostName,
		"alias":            host.Alias,
		"group_id":         host.GroupID,
		"credential_id":    host.CredentialID,
		"connection_mode":  host.ConnectionMode,
		"gateway_id":       host.GatewayID,
		"cloud_account_id": host.CloudAccountID,
		"private_ip":       host.PrivateIP,
		"public_ip":        host.PublicIP,
		"ssh_user":         host.SSHUser,
		"ssh_ip":           host.SSHIP,
		"ssh_port":         host.SSHPort,
		"os":               host.OS,
		"arch":             host.Arch,
		"cpu":              host.CPU,
		"memory":           host.Memory,
		"disk":             host.Disk,
		"environment":      host.Environment,
		"provider":         host.Provider,
		"region":           host.Region,
		"status":           host.Status,
		"description":      host.Description,
	}
}

func normalizeGroupIDs(groupIDs []uint, fallback uint) []uint {
	result := make([]uint, 0, len(groupIDs)+1)
	seen := map[uint]struct{}{}
	for _, id := range groupIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if len(result) == 0 && fallback > 0 {
		result = append(result, fallback)
	}
	return result
}

func firstGroupID(groupIDs []uint) uint {
	if len(groupIDs) > 0 {
		return groupIDs[0]
	}
	return 0
}

func syncAssetHostGroups(db *gorm.DB, hostID uint, groupIDs []uint, fallback uint) error {
	normalized := normalizeGroupIDs(groupIDs, fallback)
	if len(normalized) == 0 {
		return nil
	}
	if err := db.Where("host_id = ?", hostID).Delete(&model.AssetHostGroupRelation{}).Error; err != nil {
		return err
	}
	relations := make([]model.AssetHostGroupRelation, 0, len(normalized))
	for _, groupID := range normalized {
		relations = append(relations, model.AssetHostGroupRelation{
			HostID:  hostID,
			GroupID: groupID,
		})
	}
	return db.Create(&relations).Error
}

func ensureHostGroupFallback(host *model.AssetHost) {
	if host == nil {
		return
	}
	if len(host.HostGroups) == 0 && host.GroupID > 0 && host.Group.ID > 0 {
		host.HostGroups = []model.AssetHostGroup{host.Group}
	}
}

func ensureHostGroupFallbackList(list []model.AssetHost) {
	for index := range list {
		ensureHostGroupFallback(&list[index])
	}
}

func normalizedAuthType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "key", "private_key", "密钥认证":
		return "key"
	default:
		return "password"
	}
}

func maskCredential(item *model.AssetCredential) {
	item.Password = maskedSecret(item.Password)
	item.PrivateKey = maskedSecret(item.PrivateKey)
	item.Passphrase = maskedSecret(item.Passphrase)
}

func maskedSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "******"
}

func (s *Service) newSSHClient(host model.AssetHost) (*ssh.Client, error) {
	return s.newSSHClientWithTimeout(host, 5*time.Second)
}

func (s *Service) newSSHClientWithTimeout(host model.AssetHost, timeout time.Duration) (*ssh.Client, error) {
	if host.SSHIP == "" {
		return nil, errors.New("主机未配置 SSH 地址")
	}
	if host.SSHUser == "" {
		return nil, errors.New("主机未配置 SSH 用户")
	}
	authMethod, err := credentialAuthMethod(host.Credential)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		return nil, errors.New("主机同步超时")
	}
	config := &ssh.ClientConfig{
		User:            host.SSHUser,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	address := net.JoinHostPort(host.SSHIP, strconv.Itoa(host.SSHPort))
	if normalizeConnectionMode(host.ConnectionMode) == "gateway" && host.GatewayID != nil && *host.GatewayID > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		conn, cleanup, err := s.dialThroughGateway(ctx, *host.GatewayID, "tcp", address)
		cancel()
		if err != nil {
			return nil, err
		}
		_ = conn.SetDeadline(time.Now().Add(timeout))
		clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
		if err != nil {
			_ = conn.Close()
			cleanup()
			return nil, err
		}
		_ = conn.SetDeadline(time.Time{})
		client := ssh.NewClient(clientConn, chans, reqs)
		go func() {
			_ = clientConn.Wait()
			cleanup()
		}()
		return client, nil
	}
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func credentialAuthMethod(credential model.AssetCredential) (ssh.AuthMethod, error) {
	switch normalizedAuthType(credential.AuthType) {
	case "key":
		var signer ssh.Signer
		var err error
		if strings.TrimSpace(credential.Passphrase) != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(credential.PrivateKey), []byte(credential.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(credential.PrivateKey))
		}
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	default:
		if credential.Password == "" {
			return nil, errors.New("密码凭据为空")
		}
		return ssh.Password(credential.Password), nil
	}
}

func remainingTimeout(deadline time.Time, maximum time.Duration) time.Duration {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	if remaining < maximum {
		return remaining
	}
	return maximum
}

func collectHostInfo(client *ssh.Client, deadline time.Time) (map[string]string, bool) {
	result := map[string]string{}
	commands := map[string]string{
		"os":         ". /etc/os-release 2>/dev/null && printf '%s' \"$PRETTY_NAME\" || lsb_release -ds 2>/dev/null || uname -sr 2>/dev/null || true",
		"arch":       "uname -m 2>/dev/null || true",
		"cpu":        "nproc 2>/dev/null || grep -c processor /proc/cpuinfo 2>/dev/null || true",
		"memory":     "free -m 2>/dev/null | awk '/Mem:/ {printf \"%dG\", int(($2 + 1023) / 1024)}' || true",
		"disk":       "df -h / 2>/dev/null | awk 'NR==2 {print $2}' || true",
		"private_ip": "hostname -I 2>/dev/null | awk '{print $1}' || true",
		"public_ip":  "curl -s --max-time 2 ifconfig.me 2>/dev/null || wget -qO- -T 2 ifconfig.me 2>/dev/null || true",
	}
	for field, command := range commands {
		timeout := remainingTimeout(deadline, 5*time.Second)
		if timeout <= 0 {
			return result, true
		}
		value, timedOut := runSSHCommandWithTimeout(client, command, timeout)
		if timedOut {
			return result, true
		}
		if field == "cpu" && value != "" {
			value = value + "核"
		}
		result[field] = value
	}
	return result, false
}

func runSSHCommand(client *ssh.Client, command string) string {
	output, _ := runSSHCommandWithTimeout(client, command, 5*time.Second)
	return output
}

func runSSHCommandWithTimeout(client *ssh.Client, command string, timeout time.Duration) (string, bool) {
	session, err := client.NewSession()
	if err != nil {
		return "", false
	}
	defer session.Close()
	type commandResult struct {
		output []byte
		err    error
	}
	done := make(chan commandResult, 1)
	go func() {
		output, runErr := session.CombinedOutput(command)
		done <- commandResult{output: output, err: runErr}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			return "", false
		}
		return strings.TrimSpace(string(result.output)), false
	case <-time.After(timeout):
		_ = session.Close()
		return "", true
	}
}

func lastField(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func formatConfig(host model.AssetHost) string {
	parts := make([]string, 0, 3)
	if host.CPU != "" {
		parts = append(parts, host.CPU)
	}
	if host.Memory != "" {
		parts = append(parts, host.Memory)
	}
	if host.Disk != "" {
		parts = append(parts, host.Disk)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " / ")
}

type cloudInstance struct {
	InstanceID string
	HostName   string
	PrivateIP  string
	PublicIP   string
	CPU        string
	Memory     string
	Disk       string
	OS         string
	Region     string
	SSHUser    string
	SSHPort    int
}

func fetchCloudInstances(provider, accessKey, secretKey, region string) ([]cloudInstance, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "tencent", "tencentcloud":
		service := util.NewTencentCloudService(accessKey, secretKey)
		instances, err := service.GetInstances(normalizeCloudRegions(nil, region))
		if err != nil {
			return nil, err
		}
		result := make([]cloudInstance, 0, len(instances))
		for _, item := range instances {
			result = append(result, cloudInstance{
				InstanceID: item.InstanceID,
				HostName:   firstNonEmpty(item.HostName, item.InstanceID),
				PrivateIP:  item.PrivateIP,
				PublicIP:   item.PublicIP,
				CPU:        fmt.Sprintf("%d核", item.CPU),
				Memory:     fmt.Sprintf("%dGB", item.Memory/1024),
				Disk:       fmt.Sprintf("%dGB", item.Disk),
				OS:         item.OS,
				Region:     item.Region,
				SSHUser:    "root",
				SSHPort:    22,
			})
		}
		return result, nil
	case "aliyun", "alicloud":
		return fetchAliyunCloudInstances(accessKey, secretKey, region)
	default:
		return nil, fmt.Errorf("unsupported cloud provider: %s", provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultSSHPort(port int) int {
	if port > 0 {
		return port
	}
	return 22
}
