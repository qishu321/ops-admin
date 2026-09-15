package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/auth"
	"ops-admin/backend/httpx"
	"ops-admin/backend/model"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/xuri/excelize/v2"
)

type Controller struct {
	service *service.Service
}

const refreshCookieName = "ops-admin-refresh"

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func New(svc *service.Service) *Controller {
	return &Controller{service: svc}
}

func (ctl *Controller) Ping(c *gin.Context) {
	httpx.Success(c, gin.H{"name": "ops-admin", "status": "ok"})
}

func (ctl *Controller) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Failed(c, http.StatusBadRequest, "invalid login payload")
		return
	}
	data, refreshToken, err := ctl.service.Login(req, c.ClientIP(), c.Request.UserAgent(), runtime.GOOS)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	setRefreshCookie(c, refreshToken, sessionCookieMaxAge(data))
	httpx.Success(c, data)
}

func (ctl *Controller) RefreshToken(c *gin.Context) {
	var payload struct {
		LastActivityAt int64 `json:"lastActivityAt"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		httpx.Failed(c, http.StatusBadRequest, "invalid refresh payload")
		return
	}
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil {
		clearRefreshCookie(c)
		httpx.Failed(c, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	reportedAt := time.Time{}
	if payload.LastActivityAt > 0 {
		reportedAt = time.UnixMilli(payload.LastActivityAt)
	}
	data, nextRefreshToken, err := ctl.service.RefreshSession(refreshToken, reportedAt)
	if err != nil {
		clearRefreshCookie(c)
		httpx.Failed(c, http.StatusUnauthorized, "登录已过期，请重新登录")
		return
	}
	setRefreshCookie(c, nextRefreshToken, sessionCookieMaxAge(data))
	httpx.Success(c, data)
}

func (ctl *Controller) Logout(c *gin.Context) {
	if refreshToken, err := c.Cookie(refreshCookieName); err == nil {
		ctl.service.RevokeSession(refreshToken)
	}
	clearRefreshCookie(c)
	httpx.Success(c, gin.H{"loggedOut": true})
}

func setRefreshCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetCookie(refreshCookieName, token, maxAge, "/api/v1/auth", "", secure, true)
}

func sessionCookieMaxAge(data map[string]any) int {
	rememberLogin, _ := data["rememberLogin"].(bool)
	if !rememberLogin {
		// A zero Max-Age creates a browser-session cookie.
		return 0
	}
	expiresAt, ok := data["sessionExpiresAt"].(int64)
	if !ok {
		return int(auth.SessionMaxTTL.Seconds())
	}
	return max(0, int(time.Until(time.UnixMilli(expiresAt)).Seconds()))
}

func clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetCookie(refreshCookieName, "", -1, "/api/v1/auth", "", secure, true)
}

func (ctl *Controller) Profile(c *gin.Context) {
	userID := c.GetUint("userID")
	data, err := ctl.service.GetProfile(userID)
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetSystemConfig(c *gin.Context) {
	data, err := ctl.service.GetSystemConfig()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateSystemConfig(c *gin.Context) {
	var payload service.SystemConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid system config payload")
		return
	}
	data, err := ctl.service.UpdateSystemConfig(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UploadSystemAsset(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Failed(c, 400, "请选择要上传的文件")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
	default:
		httpx.Failed(c, 400, "仅支持图片文件上传")
		return
	}

	if err := os.MkdirAll("uploads/system", 0o755); err != nil {
		httpx.Failed(c, 500, "创建上传目录失败")
		return
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	target := filepath.Join("uploads", "system", filename)
	if err := c.SaveUploadedFile(file, target); err != nil {
		httpx.Failed(c, 500, "保存文件失败")
		return
	}

	httpx.Success(c, gin.H{
		"url": "/" + filepath.ToSlash(target),
	})
}

func (ctl *Controller) DownloadAssetHostTemplate(c *gin.Context) {
	file := excelize.NewFile()
	sheet := file.GetSheetName(0)
	headers := []string{"主机名称*", "SSH地址*", "SSH端口", "SSH用户*", "认证凭据*", "连接方式*", "访问网关", "所属环境*", "私网IP", "公网IP", "云厂商", "所在区域", "备注"}
	for idx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(idx+1, 1)
		_ = file.SetCellValue(sheet, cell, header)
	}
	_ = file.SetCellValue(sheet, "A2", "web-01")
	_ = file.SetCellValue(sheet, "B2", "192.168.101.159")
	_ = file.SetCellValue(sheet, "C2", 22)
	_ = file.SetCellValue(sheet, "D2", "root")
	_ = file.SetCellValue(sheet, "E2", "default-ssh")
	_ = file.SetCellValue(sheet, "F2", "direct")
	_ = file.SetCellValue(sheet, "H2", "production")
	_ = file.SetCellValue(sheet, "I2", "192.168.101.159")
	_ = file.SetCellValue(sheet, "K2", "自建")
	_ = file.SetCellValue(sheet, "M2", "excel导入示例")
	_ = file.SetColWidth(sheet, "A", "M", 18)
	_ = file.SetColWidth(sheet, "M", "M", 30)
	_ = file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	_, err := file.NewSheet("填写说明")
	if err == nil {
		_ = file.SetCellValue("填写说明", "A1", "字段说明")
		_ = file.SetCellValue("填写说明", "A2", "带 * 的列为必填；目标主机组由导入弹窗统一选择。")
		_ = file.SetCellValue("填写说明", "A3", "连接方式仅支持 direct（直连）或 gateway（通过网关）；gateway 时必须填写已启用的访问网关名称。")
		_ = file.SetCellValue("填写说明", "A4", "认证凭据和访问网关均按名称匹配，必须已在平台中创建且启用。")
		_ = file.SetCellValue("填写说明", "A5", "所属环境填写环境编码，例如 production、test；SSH 地址建议填写内网 IP。")
		_ = file.SetColWidth("填写说明", "A", "A", 100)
		file.SetActiveSheet(0)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		httpx.Failed(c, 500, "failed to generate template")
		return
	}
	c.Header("Content-Disposition", "attachment; filename=asset-host-template.xlsx")
	c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}

func (ctl *Controller) GetSysAdminList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListAdmins(pageNum, pageSize, c.Query("username"), c.Query("status"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetSysAdminInfo(c *gin.Context) {
	id := uint(mustAtoi(c.Query("id")))
	data, err := ctl.service.GetAdmin(id)
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateSysAdmin(c *gin.Context) {
	var payload service.AdminPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid user payload")
		return
	}
	if err := ctl.service.CreateAdmin(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateSysAdmin(c *gin.Context) {
	var payload service.AdminPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid user payload")
		return
	}
	if err := ctl.service.UpdateAdmin(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteSysAdmin(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAdmin(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateSysAdminStatus(c *gin.Context) {
	var payload service.AdminStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdateAdminStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) ResetSysAdminPassword(c *gin.Context) {
	var payload struct {
		ID       uint   `json:"id"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid password payload")
		return
	}
	if err := ctl.service.ResetAdminPassword(payload.ID, payload.Password); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdatePersonal(c *gin.Context) {
	var payload service.AdminPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid profile payload")
		return
	}
	if err := ctl.service.UpdateProfile(c.GetUint("userID"), payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdatePersonalPassword(c *gin.Context) {
	var payload struct {
		Password      string `json:"password"`
		NewPassword   string `json:"newPassword"`
		ResetPassword string `json:"resetPassword"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid password payload")
		return
	}
	if payload.NewPassword != payload.ResetPassword {
		httpx.Failed(c, 400, "new password confirmation mismatch")
		return
	}
	if err := ctl.service.UpdateProfilePassword(c.GetUint("userID"), payload.Password, payload.NewPassword); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetRoleList(c *gin.Context) {
	list, err := ctl.service.ListRoles()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, map[string]any{"list": list, "total": len(list)})
}

func (ctl *Controller) QuerySysRoleVoList(c *gin.Context) {
	list, err := ctl.service.ListRoles()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	vo := make([]gin.H, 0, len(list))
	for _, item := range list {
		vo = append(vo, gin.H{"id": item.ID, "roleName": item.RoleName})
	}
	httpx.Success(c, vo)
}

func (ctl *Controller) CreateRole(c *gin.Context) {
	var payload service.RolePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid role payload")
		return
	}
	if err := ctl.service.CreateRole(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateRole(c *gin.Context) {
	var payload service.RolePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid role payload")
		return
	}
	if err := ctl.service.UpdateRole(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteRole(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteRole(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetRoleInfo(c *gin.Context) {
	role, err := ctl.service.GetRole(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, role)
}

func (ctl *Controller) UpdateRoleStatus(c *gin.Context) {
	var payload service.RoleStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdateRoleStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) QueryRoleMenuIDList(c *gin.Context) {
	ids, err := ctl.service.RoleMenuIDs(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	result := make([]gin.H, 0, len(ids))
	for _, id := range ids {
		result = append(result, gin.H{"id": id})
	}
	httpx.Success(c, result)
}

func (ctl *Controller) AssignPermissions(c *gin.Context) {
	var payload service.RoleMenuPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid permissions payload")
		return
	}
	if err := ctl.service.AssignRoleMenus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMenuList(c *gin.Context) {
	list, err := ctl.service.ListMenus()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, list)
}

func (ctl *Controller) QuerySysMenuVoList(c *gin.Context) {
	list, err := ctl.service.ListMenus()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	vo := make([]gin.H, 0, len(list))
	for _, item := range list {
		vo = append(vo, gin.H{"id": item.ID, "parentId": item.ParentID, "label": item.MenuName})
	}
	httpx.Success(c, vo)
}

func (ctl *Controller) CreateMenu(c *gin.Context) {
	var payload service.MenuPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid menu payload")
		return
	}
	if err := ctl.service.CreateMenu(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateMenu(c *gin.Context) {
	var payload service.MenuPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid menu payload")
		return
	}
	if err := ctl.service.UpdateMenu(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteMenu(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteMenu(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetMenuInfo(c *gin.Context) {
	menu, err := ctl.service.GetMenu(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, menu)
}

func (ctl *Controller) GetDeptList(c *gin.Context) {
	list, err := ctl.service.ListDepts()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, list)
}

func (ctl *Controller) QuerySysDeptVoList(c *gin.Context) {
	list, err := ctl.service.ListDepts()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	vo := make([]gin.H, 0, len(list))
	for _, item := range list {
		vo = append(vo, gin.H{"id": item.ID, "parentId": item.ParentID, "label": item.DeptName})
	}
	httpx.Success(c, vo)
}

func (ctl *Controller) CreateDept(c *gin.Context) {
	var payload service.DeptPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid department payload")
		return
	}
	if err := ctl.service.CreateDept(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateDept(c *gin.Context) {
	var payload service.DeptPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid department payload")
		return
	}
	if err := ctl.service.UpdateDept(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteDept(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteDept(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetDeptInfo(c *gin.Context) {
	dept, err := ctl.service.GetDept(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, dept)
}

func (ctl *Controller) GetDeptUsers(c *gin.Context) {
	deptID := uint(mustAtoi(c.Query("deptId")))
	users, err := ctl.service.DeptUsers(deptID)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, users)
}

func (ctl *Controller) GetPostList(c *gin.Context) {
	list, err := ctl.service.ListPosts()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, map[string]any{"list": list, "total": len(list)})
}

func (ctl *Controller) QuerySysPostVoList(c *gin.Context) {
	list, err := ctl.service.ListPosts()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	vo := make([]gin.H, 0, len(list))
	for _, item := range list {
		vo = append(vo, gin.H{"id": item.ID, "postName": item.PostName})
	}
	httpx.Success(c, vo)
}

func (ctl *Controller) CreatePost(c *gin.Context) {
	var payload service.PostPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid post payload")
		return
	}
	if err := ctl.service.CreatePost(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdatePost(c *gin.Context) {
	var payload service.PostPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid post payload")
		return
	}
	if err := ctl.service.UpdatePost(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeletePost(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeletePost(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchDeletePost(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch delete payload")
		return
	}
	if err := ctl.service.BatchDeletePosts(payload.IDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetPostInfo(c *gin.Context) {
	post, err := ctl.service.GetPost(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, post)
}

func (ctl *Controller) UpdatePostStatus(c *gin.Context) {
	var payload service.PostStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid status payload")
		return
	}
	if err := ctl.service.UpdatePostStatus(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetLoginLogList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListLoginLogs(pageNum, pageSize, c.Query("username"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteLoginLog(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteLoginLog(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchDeleteLoginLog(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch delete payload")
		return
	}
	if err := ctl.service.BatchDeleteLoginLogs(payload.IDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) CleanLoginLog(c *gin.Context) {
	if err := ctl.service.CleanLoginLogs(); err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetOperationLogList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListOperationLogs(pageNum, pageSize, c.Query("username"), c.Query("keyword"), c.Query("riskLevel"), c.Query("success"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteOperationLog(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteOperationLog(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchDeleteOperationLog(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch delete payload")
		return
	}
	if err := ctl.service.BatchDeleteOperationLogs(payload.IDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) CleanOperationLog(c *gin.Context) {
	if err := ctl.service.CleanOperationLogs(); err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetAssetHostList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListAssetHosts(pageNum, pageSize, c.Query("keyword"), uint(mustAtoi(c.Query("groupId"))), c.Query("status"), c.Query("environment"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetOverview(c *gin.Context) {
	data, err := ctl.service.GetAssetOverview()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetChangeLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	data, err := ctl.service.ListAssetChangeLogs(c.Query("resourceType"), uint(mustAtoi(c.Query("resourceId"))), limit)
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetHostInfo(c *gin.Context) {
	data, err := ctl.service.GetAssetHost(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetHostMetrics(c *gin.Context) {
	data, err := ctl.service.GetAssetHostMetrics(uint(mustAtoi(c.Query("id"))), c.DefaultQuery("range", "1h"), c.Query("start"), c.Query("end"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateAssetHost(c *gin.Context) {
	var payload service.AssetHostPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid host payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.CreateAssetHost(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateAssetHost(c *gin.Context) {
	var payload service.AssetHostPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid host payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.UpdateAssetHost(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) ImportAssetHosts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Failed(c, 400, "please upload excel file")
		return
	}
	groupID := uint(mustAtoi(c.PostForm("groupId")))
	if groupID == 0 {
		httpx.Failed(c, 400, "groupId is required")
		return
	}
	if err := os.MkdirAll("uploads/temp", 0o755); err != nil {
		httpx.Failed(c, 500, "failed to create temp dir")
		return
	}
	tempPath := filepath.Join("uploads", "temp", fmt.Sprintf("asset-host-import-%d%s", time.Now().UnixNano(), filepath.Ext(file.Filename)))
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		httpx.Failed(c, 500, "failed to save excel file")
		return
	}
	defer os.Remove(tempPath)

	workbook, err := excelize.OpenFile(tempPath)
	if err != nil {
		httpx.Failed(c, 400, "failed to parse excel file")
		return
	}
	defer workbook.Close()

	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		httpx.Failed(c, 400, "excel sheet is empty")
		return
	}
	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		httpx.Failed(c, 400, "failed to read excel rows")
		return
	}
	if len(rows) == 0 || len(rows[0]) < 8 {
		httpx.Failed(c, 400, "Excel 模板格式已更新，请下载最新模板后导入")
		return
	}

	importRows := make([]service.AssetHostImportRow, 0)
	for index, row := range rows {
		if index == 0 {
			continue
		}
		if len(row) < 8 {
			continue
		}
		importRows = append(importRows, service.AssetHostImportRow{
			HostName:       strings.TrimSpace(row[0]),
			SSHIP:          strings.TrimSpace(row[1]),
			SSHPort:        mustAtoi(strings.TrimSpace(row[2])),
			SSHUser:        strings.TrimSpace(row[3]),
			CredentialName: strings.TrimSpace(row[4]),
			ConnectionMode: strings.TrimSpace(row[5]),
			GatewayName:    safeExcelCell(row, 6),
			Environment:    safeExcelCell(row, 7),
			PrivateIP:      safeExcelCell(row, 8),
			PublicIP:       safeExcelCell(row, 9),
			Provider:       safeExcelCell(row, 10),
			Region:         safeExcelCell(row, 11),
			Description:    safeExcelCell(row, 12),
		})
	}

	data, err := ctl.service.ImportAssetHosts(groupID, importRows)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SyncAssetHostsFromCloud(c *gin.Context) {
	var payload service.AssetCloudSyncPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid cloud sync payload")
		return
	}
	data, err := ctl.service.SyncAssetHostsFromCloud(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) SyncAssetHost(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid sync payload")
		return
	}
	data, err := ctl.service.SyncAssetHost(payload.ID)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) BatchSyncAssetHosts(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch sync payload")
		return
	}
	data, err := ctl.service.BatchSyncAssetHosts(payload.IDs)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) AssetTerminalWS(c *gin.Context) {
	token := c.Query("token")
	if strings.TrimSpace(token) == "" {
		httpx.Failed(c, http.StatusUnauthorized, "请先登录")
		return
	}
	if _, err := auth.ParseToken(token); err != nil {
		httpx.Failed(c, http.StatusUnauthorized, auth.TokenErrorMessage(err))
		return
	}

	hostID := uint(mustAtoi(c.Query("hostId")))
	rows := mustAtoi(c.DefaultQuery("rows", "30"))
	cols := mustAtoi(c.DefaultQuery("cols", "120"))
	terminal, err := ctl.service.OpenAssetTerminal(hostID, rows, cols)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	defer terminal.Session.Close()
	defer terminal.Client.Close()

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	writePipe := func(reader io.Reader) {
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = conn.WriteMessage(websocket.TextMessage, buf[:n])
				writeMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}
	go writePipe(terminal.Stdout)
	go writePipe(terminal.Stderr)
	go func() {
		_ = terminal.Session.Wait()
		_ = conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if _, err := terminal.Stdin.Write(message); err != nil {
			return
		}
	}
}

func (ctl *Controller) DeleteAssetHost(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAssetHost(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchDeleteAssetHosts(c *gin.Context) {
	var payload service.BatchIDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch delete payload")
		return
	}
	if err := ctl.service.BatchDeleteAssetHosts(payload.IDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) RemoveAssetHostsFromGroup(c *gin.Context) {
	var payload service.AssetHostGroupMemberPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid remove group member payload")
		return
	}
	hostIDs := payload.HostIDs
	if payload.HostID > 0 {
		hostIDs = append(hostIDs, payload.HostID)
	}
	if err := ctl.service.RemoveAssetHostsFromGroup(payload.GroupID, hostIDs); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) BatchReplaceAssetHostCredential(c *gin.Context) {
	var payload service.AssetHostBatchCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch credential payload")
		return
	}
	if err := ctl.service.BatchReplaceAssetHostCredential(payload.IDs, payload.CredentialID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetAssetHostGroupList(c *gin.Context) {
	data, err := ctl.service.ListAssetHostGroups(c.Query("keyword"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetHostGroupInfo(c *gin.Context) {
	data, err := ctl.service.GetAssetHostGroup(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateAssetHostGroup(c *gin.Context) {
	var payload service.AssetHostGroupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid host group payload")
		return
	}
	if err := ctl.service.CreateAssetHostGroup(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateAssetHostGroup(c *gin.Context) {
	var payload service.AssetHostGroupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid host group payload")
		return
	}
	if err := ctl.service.UpdateAssetHostGroup(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteAssetHostGroup(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAssetHostGroup(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetAssetCredentialList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListAssetCredentials(pageNum, pageSize, c.Query("keyword"), c.Query("authType"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetCredentialOptions(c *gin.Context) {
	data, err := ctl.service.ListAssetCredentialOptions()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetCredentialInfo(c *gin.Context) {
	data, err := ctl.service.GetAssetCredential(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateAssetCredential(c *gin.Context) {
	var payload service.AssetCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid credential payload")
		return
	}
	if err := ctl.service.CreateAssetCredential(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateAssetCredential(c *gin.Context) {
	var payload service.AssetCredentialPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid credential payload")
		return
	}
	if err := ctl.service.UpdateAssetCredential(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteAssetCredential(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAssetCredential(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) GetAssetCloudAccountList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListAssetCloudAccounts(pageNum, pageSize, c.Query("keyword"), c.Query("provider"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetCloudAccountOptions(c *gin.Context) {
	data, err := ctl.service.ListAssetCloudAccountOptions()
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetCloudAccountInfo(c *gin.Context) {
	data, err := ctl.service.GetAssetCloudAccount(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateAssetCloudAccount(c *gin.Context) {
	var payload service.AssetCloudAccountPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid cloud account payload")
		return
	}
	if err := ctl.service.CreateAssetCloudAccount(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateAssetCloudAccount(c *gin.Context) {
	var payload service.AssetCloudAccountPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid cloud account payload")
		return
	}
	if err := ctl.service.UpdateAssetCloudAccount(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteAssetCloudAccount(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAssetCloudAccount(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func mustAtoi(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

func safeExcelCell(row []string, index int) string {
	if index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}

func BuildMenuTree(list []model.Menu) []gin.H {
	nodes := make(map[uint]gin.H, len(list))
	result := make([]gin.H, 0)
	for _, item := range list {
		node := gin.H{
			"id":         item.ID,
			"parentId":   item.ParentID,
			"menuName":   item.MenuName,
			"icon":       item.Icon,
			"value":      item.Value,
			"menuType":   item.MenuType,
			"url":        item.URL,
			"menuStatus": item.MenuStatus,
			"sort":       item.Sort,
			"createTime": item.CreatedAt,
			"children":   []gin.H{},
		}
		nodes[item.ID] = node
	}
	for _, item := range list {
		if item.ParentID == 0 {
			result = append(result, nodes[item.ID])
			continue
		}
		parent := nodes[item.ParentID]
		if parent == nil {
			continue
		}
		parentChildren := parent["children"].([]gin.H)
		parentChildren = append(parentChildren, nodes[item.ID])
		parent["children"] = parentChildren
		nodes[item.ParentID] = parent
	}
	return result
}
