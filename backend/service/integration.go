package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"ops-admin/backend/model"

	"gorm.io/gorm"
)

type IntegrationNavigationGroupPayload struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	IsPublic      bool      `json:"isPublic"`
	Status        int       `json:"status"`
	Sort          int       `json:"sort"`
	PublicOrigins *[]string `json:"publicOrigins"`
}

type IntegrationNavigationPayload struct {
	ID          uint   `json:"id"`
	GroupID     uint   `json:"groupId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	IconURL     string `json:"iconUrl"`
	OpenMode    string `json:"openMode"`
	Status      int    `json:"status"`
	Sort        int    `json:"sort"`
}

func (s *Service) ListIntegrationNavigationGroups(keyword string) ([]map[string]any, error) {
	query := s.db.Model(&model.IntegrationNavigationGroup{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ? OR description LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	var groups []model.IntegrationNavigationGroup
	if err := query.Order("sort ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		var count int64
		if err := s.db.Model(&model.IntegrationNavigation{}).Where("group_id = ?", group.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id": group.ID, "name": group.Name, "description": group.Description,
			"isPublic": group.IsPublic, "publicToken": group.PublicToken, "publicOrigins": group.PublicOrigins,
			"status": group.Status, "sort": group.Sort, "itemCount": count,
			"createTime": group.CreatedAt, "updateTime": group.UpdatedAt,
		})
	}
	return result, nil
}

func (s *Service) SaveIntegrationNavigationGroup(payload IntegrationNavigationGroupPayload) (model.IntegrationNavigationGroup, error) {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return model.IntegrationNavigationGroup{}, errors.New("导航组名称不能为空")
	}
	status := payload.Status
	if status != 2 {
		status = 1
	}
	var group model.IntegrationNavigationGroup
	if payload.ID > 0 {
		if err := s.db.First(&group, payload.ID).Error; err != nil {
			return group, err
		}
	}
	group.Name = name
	group.Description = strings.TrimSpace(payload.Description)
	group.IsPublic = payload.IsPublic
	group.Status = status
	group.Sort = payload.Sort
	if payload.PublicOrigins != nil {
		origins, err := normalizeNavigationPublicOrigins(*payload.PublicOrigins)
		if err != nil {
			return group, err
		}
		group.PublicOrigins = origins
	}
	if group.IsPublic && group.PublicToken == "" {
		token, err := newPublicNavigationToken()
		if err != nil {
			return group, err
		}
		group.PublicToken = token
	}
	if err := s.db.Save(&group).Error; err != nil {
		return group, err
	}
	return group, nil
}

func (s *Service) DeleteIntegrationNavigationGroup(id uint) error {
	if id == 0 {
		return errors.New("导航组 ID 不能为空")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&model.IntegrationNavigation{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&model.IntegrationNavigationGroup{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Service) RegenerateIntegrationPublicToken(id uint) (model.IntegrationNavigationGroup, error) {
	var group model.IntegrationNavigationGroup
	if err := s.db.First(&group, id).Error; err != nil {
		return group, err
	}
	token, err := newPublicNavigationToken()
	if err != nil {
		return group, err
	}
	group.PublicToken = token
	group.IsPublic = true
	if err := s.db.Save(&group).Error; err != nil {
		return group, err
	}
	return group, nil
}

func (s *Service) ListIntegrationNavigations(groupID uint, keyword string) ([]model.IntegrationNavigation, error) {
	query := s.db.Model(&model.IntegrationNavigation{})
	if groupID > 0 {
		query = query.Where("group_id = ?", groupID)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ? OR description LIKE ? OR url LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	var items []model.IntegrationNavigation
	err := query.Order("sort ASC, id ASC").Find(&items).Error
	return items, err
}

func (s *Service) SaveIntegrationNavigation(payload IntegrationNavigationPayload) (model.IntegrationNavigation, error) {
	name := strings.TrimSpace(payload.Name)
	targetURL := strings.TrimSpace(payload.URL)
	if name == "" || payload.GroupID == 0 || targetURL == "" {
		return model.IntegrationNavigation{}, errors.New("导航名称、所属分组和访问地址不能为空")
	}
	if err := validatePublicNavigationURL(targetURL); err != nil {
		return model.IntegrationNavigation{}, err
	}
	iconURL := strings.TrimSpace(payload.IconURL)
	if iconURL != "" {
		if err := validatePublicNavigationURL(iconURL); err != nil {
			return model.IntegrationNavigation{}, errors.New("图标地址必须是有效的 HTTP/HTTPS 地址")
		}
	}
	var group model.IntegrationNavigationGroup
	if err := s.db.First(&group, payload.GroupID).Error; err != nil {
		return model.IntegrationNavigation{}, errors.New("所属导航组不存在")
	}
	status := payload.Status
	if status != 2 {
		status = 1
	}
	openMode := payload.OpenMode
	if openMode != "current" {
		openMode = "new"
	}
	var item model.IntegrationNavigation
	if payload.ID > 0 {
		if err := s.db.First(&item, payload.ID).Error; err != nil {
			return item, err
		}
	}
	item.GroupID = payload.GroupID
	item.Name = name
	item.Description = strings.TrimSpace(payload.Description)
	item.URL = targetURL
	item.IconURL = iconURL
	item.OpenMode = openMode
	item.Status = status
	item.Sort = payload.Sort
	if err := s.db.Save(&item).Error; err != nil {
		return item, err
	}
	return item, nil
}

func (s *Service) DeleteIntegrationNavigation(id uint) error {
	if id == 0 {
		return errors.New("导航 ID 不能为空")
	}
	result := s.db.Delete(&model.IntegrationNavigation{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) GetPublicIntegrationNavigation(token string) (map[string]any, error) {
	var group model.IntegrationNavigationGroup
	if err := s.db.Where("public_token = ? AND is_public = ? AND status = ?", strings.TrimSpace(token), true, 1).First(&group).Error; err != nil {
		return nil, err
	}
	var items []model.IntegrationNavigation
	if err := s.db.Where("group_id = ? AND status = ?", group.ID, 1).Order("sort ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	publicItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		publicItems = append(publicItems, map[string]any{
			"name": item.Name, "description": item.Description, "url": item.URL,
			"iconUrl": item.IconURL, "openMode": item.OpenMode,
		})
	}
	return map[string]any{
		"name": group.Name, "description": group.Description, "items": publicItems,
	}, nil
}

func newPublicNavigationToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func normalizeNavigationPublicOrigins(values []string) ([]string, error) {
	if len(values) > 10 {
		return nil, errors.New("最多配置 10 个访问入口")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 2048 {
			return nil, errors.New("入口地址不能超过 2048 字节")
		}
		u, err := url.Parse(value)
		if err != nil {
			return nil, errors.New("入口地址必须是有效的 HTTP/HTTPS 地址")
		}
		scheme := strings.ToLower(u.Scheme)
		if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(value, "#") {
			return nil, errors.New("入口地址需包含 http:// 或 https://，不能包含账号、查询参数或片段")
		}
		path := strings.Trim(u.Path, "/")
		parts := strings.Split(path, "/")
		if path != "" && !(len(parts) == 3 && parts[0] == "public" && parts[1] == "navigation" && parts[2] != "") {
			return nil, errors.New("请填写平台 IP/域名和端口，或粘贴完整的公开导航链接")
		}
		host, port := strings.ToLower(u.Hostname()), u.Port()
		if port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return nil, errors.New("入口地址端口必须在 1 到 65535 之间")
			}
		}
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		if port != "" && !(scheme == "http" && port == "80" || scheme == "https" && port == "443") {
			host += ":" + port
		}
		origin := scheme + "://" + host
		if !seen[origin] {
			result = append(result, origin)
			seen[origin] = true
		}
	}
	return result, nil
}

func validatePublicNavigationURL(value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("访问地址必须是有效的 HTTP/HTTPS 地址")
	}
	return nil
}
