package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/model"

	"gorm.io/gorm"
)

const (
	notifyStatusPending  = "pending"
	notifyStatusSending  = "sending"
	notifyStatusRetrying = "retrying"
	notifyStatusSuccess  = "success"
	notifyStatusFailed   = "failed"
	defaultNotifyRetries = 3
)

var notifyTemplateVariablePattern = regexp.MustCompile(`{{\s*[^{}]+\s*}}`)

type NotifyTemplatePayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	ChannelType string `json:"channelType"`
	Scope       string `json:"scope"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type NotifyChannelPayload struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	ChannelType string `json:"channelType"`
	WebhookURL  string `json:"webhookUrl"`
	Secret      string `json:"secret"`
	HeadersJSON string `json:"headersJson"`
	Status      int    `json:"status"`
	Description string `json:"description"`
}

type NotifyRulePayload struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Scope       string   `json:"scope"`
	Events      []string `json:"events"`
	TemplateID  uint     `json:"templateId"`
	ChannelIDs  []uint   `json:"channelIds"`
	Status      int      `json:"status"`
	Description string   `json:"description"`
}

type NotifyEvent struct {
	Scope      string
	Event      string
	TargetID   uint
	TargetName string
	Status     string
	Summary    string
	Detail     string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Extra      map[string]string
}

func normalizeNotifyStatus(value int) int {
	if value == 2 {
		return 2
	}
	return 1
}

func normalizeNotifyChannelType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dingtalk", "ding_talk", "dingding":
		return "dingtalk"
	case "wecom", "wechat", "wechat_work":
		return "wecom"
	case "feishu", "lark":
		return "feishu"
	default:
		return "webhook"
	}
}

func normalizeNotifyScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "schedule", "cron":
		return "schedule"
	case "job":
		return "job"
	case "pipeline", "cicd", "ci/cd":
		return "pipeline"
	case "monitor", "alert":
		return "monitor"
	default:
		return "all"
	}
}

func encodeStringList(list []string) string {
	filtered := make([]string, 0, len(list))
	for _, item := range list {
		value := strings.TrimSpace(item)
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	data, _ := json.Marshal(filtered)
	return string(data)
}

func decodeStringList(raw string) []string {
	var list []string
	if strings.TrimSpace(raw) == "" {
		return list
	}
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

func normalizeNotifyEvents(events []string, scope string) []string {
	seen := make(map[string]struct{}, len(events))
	result := make([]string, 0, len(events))
	for _, item := range events {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if value == "all" {
			return []string{"all"}
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) > 0 {
		return result
	}
	switch normalizeNotifyScope(scope) {
	case "monitor":
		return []string{"firing", "recovered"}
	case "schedule":
		return []string{"success", "failed", "recovered"}
	case "job":
		return []string{"failed", "waiting_approval", "rejected"}
	case "pipeline":
		return []string{"success", "failed", "waiting_approval", "rejected"}
	default:
		return []string{"success", "failed"}
	}
}

func normalizeNotifyHeaders(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "{}", nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(value), &headers); err != nil {
		return "", errors.New("headers must be valid JSON object")
	}
	data, _ := json.Marshal(headers)
	return string(data), nil
}

func mapNotifyTemplate(item model.NotifyTemplate) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"channelType": item.ChannelType,
		"scope":       normalizeNotifyScope(item.Scope),
		"title":       item.Title,
		"content":     item.Content,
		"status":      item.Status,
		"description": item.Description,
		"createTime":  item.CreatedAt,
		"updateTime":  item.UpdatedAt,
	}
}

func mapNotifyChannel(item model.NotifyChannel) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"channelType": item.ChannelType,
		"webhookUrl":  item.WebhookURL,
		"secret":      item.Secret,
		"headersJson": firstNonEmpty(item.HeadersJSON, "{}"),
		"status":      item.Status,
		"description": item.Description,
		"createTime":  item.CreatedAt,
		"updateTime":  item.UpdatedAt,
	}
}

func mapNotifyRule(item model.NotifyRule) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"scope":       item.Scope,
		"events":      decodeStringList(item.EventsJSON),
		"eventsJson":  firstNonEmpty(item.EventsJSON, "[]"),
		"templateId":  item.TemplateID,
		"channelIds":  decodeUintList(item.ChannelIDsJSON),
		"status":      item.Status,
		"description": item.Description,
		"createTime":  item.CreatedAt,
		"updateTime":  item.UpdatedAt,
	}
}

func (s *Service) ListNotifyTemplates(pageNum, pageSize int, keyword, channelType, scope, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.NotifyTemplate{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR title LIKE ? OR description LIKE ?", like, like, like)
	}
	if strings.TrimSpace(channelType) != "" {
		query = query.Where("channel_type = ?", normalizeNotifyChannelType(channelType))
	}
	if strings.TrimSpace(scope) != "" {
		query = query.Where("scope = ?", normalizeNotifyScope(scope))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.NotifyTemplate
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapNotifyTemplate(item))
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListNotifyTemplateOptions(channelType, scope string) ([]model.NotifyTemplate, error) {
	query := s.db.Where("status = ?", 1)
	if strings.TrimSpace(channelType) != "" {
		query = query.Where("channel_type IN ?", []string{normalizeNotifyChannelType(channelType), "webhook"})
	}
	if strings.TrimSpace(scope) != "" {
		normalizedScope := normalizeNotifyScope(scope)
		query = query.Where("scope IN ? OR scope = '' OR scope IS NULL", []string{normalizedScope, "all"})
	}
	var list []model.NotifyTemplate
	if err := query.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetNotifyTemplate(id uint) (map[string]any, error) {
	var item model.NotifyTemplate
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return mapNotifyTemplate(item), nil
}

func (s *Service) SaveNotifyTemplate(payload NotifyTemplatePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("template name is required")
	}
	if strings.TrimSpace(payload.Content) == "" {
		return errors.New("template content is required")
	}
	updates := map[string]any{
		"name":         Trimmed(payload.Name),
		"channel_type": normalizeNotifyChannelType(payload.ChannelType),
		"scope":        normalizeNotifyScope(payload.Scope),
		"title":        Trimmed(payload.Title),
		"content":      payload.Content,
		"status":       normalizeNotifyStatus(payload.Status),
		"description":  Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.NotifyTemplate{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.NotifyTemplate{}).Create(updates).Error
}

func (s *Service) DeleteNotifyTemplate(id uint) error {
	var count int64
	if err := s.db.Model(&model.NotifyRule{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("template is used by notification rules")
	}
	return s.db.Delete(&model.NotifyTemplate{}, id).Error
}

func (s *Service) ListNotifyChannels(pageNum, pageSize int, keyword, channelType, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.NotifyChannel{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if strings.TrimSpace(channelType) != "" {
		query = query.Where("channel_type = ?", normalizeNotifyChannelType(channelType))
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.NotifyChannel
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapNotifyChannel(item))
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListNotifyChannelOptions(channelType string) ([]model.NotifyChannel, error) {
	query := s.db.Where("status = ?", 1)
	if strings.TrimSpace(channelType) != "" {
		query = query.Where("channel_type = ?", normalizeNotifyChannelType(channelType))
	}
	var list []model.NotifyChannel
	if err := query.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetNotifyChannel(id uint) (map[string]any, error) {
	var item model.NotifyChannel
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return mapNotifyChannel(item), nil
}

func (s *Service) SaveNotifyChannel(payload NotifyChannelPayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("channel name is required")
	}
	if strings.TrimSpace(payload.WebhookURL) == "" {
		return errors.New("webhook url is required")
	}
	headersJSON, err := normalizeNotifyHeaders(payload.HeadersJSON)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"name":         Trimmed(payload.Name),
		"channel_type": normalizeNotifyChannelType(payload.ChannelType),
		"webhook_url":  strings.TrimSpace(payload.WebhookURL),
		"secret":       strings.TrimSpace(payload.Secret),
		"headers_json": headersJSON,
		"status":       normalizeNotifyStatus(payload.Status),
		"description":  Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.NotifyChannel{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.NotifyChannel{}).Create(updates).Error
}

func (s *Service) DeleteNotifyChannel(id uint) error {
	var rules []model.NotifyRule
	if err := s.db.Select("id", "name", "channel_ids_json").Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		for _, channelID := range decodeUintList(rule.ChannelIDsJSON) {
			if channelID == id {
				return fmt.Errorf("通知媒介正在被规则「%s」使用，请先调整规则", rule.Name)
			}
		}
	}
	return s.db.Delete(&model.NotifyChannel{}, id).Error
}

func (s *Service) ListNotifyRules(pageNum, pageSize int, keyword, scope, status string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.NotifyRule{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if strings.TrimSpace(scope) != "" {
		query = query.Where("scope IN ?", []string{normalizeNotifyScope(scope), "all"})
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.NotifyRule
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapNotifyRule(item))
	}
	return map[string]any{"list": rows, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func (s *Service) ListNotifyRuleOptions(scope string) ([]map[string]any, error) {
	query := s.db.Where("status = ?", 1)
	if strings.TrimSpace(scope) != "" {
		query = query.Where("scope IN ?", []string{normalizeNotifyScope(scope), "all"})
	}
	var list []model.NotifyRule
	if err := query.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(list))
	for _, item := range list {
		rows = append(rows, mapNotifyRule(item))
	}
	return rows, nil
}

func (s *Service) GetNotifyRule(id uint) (map[string]any, error) {
	var item model.NotifyRule
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return mapNotifyRule(item), nil
}

func (s *Service) SaveNotifyRule(payload NotifyRulePayload) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.New("rule name is required")
	}
	if payload.TemplateID == 0 {
		return errors.New("notification template is required")
	}
	if len(payload.ChannelIDs) == 0 {
		return errors.New("notification channel is required")
	}
	var tmpl model.NotifyTemplate
	if err := s.db.First(&tmpl, payload.TemplateID).Error; err != nil {
		return errors.New("所选消息模板不存在")
	}
	ruleScope := normalizeNotifyScope(payload.Scope)
	if !notifyTemplateScopeCompatible(tmpl.Scope, ruleScope) {
		return fmt.Errorf("消息模板适用于%s，不能用于%s通知规则", notifyScopeLabel(tmpl.Scope), notifyScopeLabel(ruleScope))
	}
	var channels []model.NotifyChannel
	if err := s.db.Where("id IN ?", payload.ChannelIDs).Find(&channels).Error; err != nil {
		return err
	}
	if len(channels) != len(payload.ChannelIDs) {
		return errors.New("部分通知媒介不存在，请重新选择")
	}
	templateType := normalizeNotifyChannelType(tmpl.ChannelType)
	for _, channel := range channels {
		if normalizeNotifyChannelType(channel.ChannelType) != templateType {
			return fmt.Errorf("消息模板类型为 %s，不能发送到媒介「%s」(%s)", templateType, channel.Name, channel.ChannelType)
		}
	}
	events := normalizeNotifyEvents(payload.Events, payload.Scope)
	updates := map[string]any{
		"name":             Trimmed(payload.Name),
		"scope":            ruleScope,
		"events_json":      encodeStringList(events),
		"template_id":      payload.TemplateID,
		"channel_ids_json": encodeUintList(payload.ChannelIDs),
		"status":           normalizeNotifyStatus(payload.Status),
		"description":      Trimmed(payload.Description),
	}
	if payload.ID > 0 {
		return s.db.Model(&model.NotifyRule{}).Where("id = ?", payload.ID).Updates(updates).Error
	}
	return s.db.Model(&model.NotifyRule{}).Create(updates).Error
}

func (s *Service) DeleteNotifyRule(id uint) error {
	return s.db.Delete(&model.NotifyRule{}, id).Error
}

func (s *Service) DispatchNotifyRule(ruleID uint, event NotifyEvent) {
	_, _ = s.enqueueNotifyRule(ruleID, event, false)
}

func (s *Service) TestNotifyRule(ruleID uint) (map[string]any, error) {
	now := time.Now()
	var rule model.NotifyRule
	if err := s.db.First(&rule, ruleID).Error; err != nil {
		return nil, err
	}
	count, err := s.enqueueNotifyRule(ruleID, NotifyEvent{
		Scope:      normalizeNotifyScope(rule.Scope),
		Event:      "notify",
		TargetID:   ruleID,
		TargetName: "通知规则测试",
		Status:     "firing",
		Summary:    "这是一条由 Ops Admin 发起的通知规则测试消息",
		Detail:     "如果你收到此消息，说明模板、通知媒介和持久化投递链路工作正常。",
		StartedAt:  &now,
		Extra:      map[string]string{"operator": "系统管理员"},
	}, true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"queued": count}, nil
}

func (s *Service) enqueueNotifyRule(ruleID uint, event NotifyEvent, allowDisabledRule bool) (int, error) {
	if ruleID == 0 {
		return 0, errors.New("通知规则不能为空")
	}
	var rule model.NotifyRule
	if err := s.db.First(&rule, ruleID).Error; err != nil {
		return 0, err
	}
	if !allowDisabledRule && rule.Status != 1 {
		return 0, nil
	}
	if rule.Scope != "all" && rule.Scope != normalizeNotifyScope(event.Scope) {
		return 0, nil
	}
	if event.Event != "notify" {
		events := decodeStringList(rule.EventsJSON)
		if !notifyRuleAcceptsEvent(events, event) {
			return 0, nil
		}
	}

	var tmpl model.NotifyTemplate
	if err := s.db.First(&tmpl, rule.TemplateID).Error; err != nil {
		return 0, fmt.Errorf("读取消息模板失败: %w", err)
	}
	if tmpl.Status != 1 {
		return 0, errors.New("消息模板已禁用")
	}
	if !notifyTemplateScopeCompatible(tmpl.Scope, event.Scope) {
		return 0, fmt.Errorf("消息模板适用于%s，不能处理%s事件", notifyScopeLabel(tmpl.Scope), notifyScopeLabel(event.Scope))
	}

	ids := decodeUintList(rule.ChannelIDsJSON)
	if len(ids) == 0 {
		return 0, errors.New("通知规则未配置通知媒介")
	}
	var channels []model.NotifyChannel
	if err := s.db.Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return 0, err
	}
	if len(channels) == 0 {
		return 0, errors.New("通知规则关联的媒介不存在")
	}

	now := time.Now()
	queued := 0
	for _, channel := range channels {
		titleTemplate, contentTemplate := normalizeNotifyTemplateForEvent(firstNonEmpty(tmpl.Title, event.TargetName), tmpl.Content, event)
		title := renderNotifyTemplate(titleTemplate, event)
		content := renderNotifyTemplate(contentTemplate, event)
		body, buildErr := buildNotifyBody(channel.ChannelType, title, content, event)
		status := notifyStatusPending
		errorText := ""
		if channel.Status != 1 {
			status = notifyStatusFailed
			errorText = "通知媒介已禁用"
		} else if normalizeNotifyChannelType(channel.ChannelType) != normalizeNotifyChannelType(tmpl.ChannelType) {
			status = notifyStatusFailed
			errorText = "消息模板与通知媒介类型不兼容"
		} else if buildErr != nil {
			status = notifyStatusFailed
			errorText = buildErr.Error()
		}
		item := model.NotifySendLog{
			DeliveryID: newNotifyDeliveryID(),
			RuleID:     rule.ID, RuleName: rule.Name,
			ChannelID: channel.ID, ChannelName: channel.Name, ChannelType: channel.ChannelType,
			Event: event.Event, Scope: firstNonEmpty(event.Scope, rule.Scope),
			TargetID: event.TargetID, TargetName: event.TargetName, Summary: event.Summary,
			Status: status, MaxAttempts: defaultNotifyRetries,
			NextRetryAt: &now, RequestBody: string(body), ErrorText: errorText,
		}
		if err := s.db.Create(&item).Error; err != nil {
			return queued, err
		}
		if status == notifyStatusPending {
			queued++
		}
	}
	return queued, nil
}

func notifyTemplateScopeCompatible(templateScope, targetScope string) bool {
	templateScope = normalizeNotifyScope(templateScope)
	targetScope = normalizeNotifyScope(targetScope)
	if templateScope == "all" {
		return true
	}
	return targetScope != "all" && templateScope == targetScope
}

func notifyScopeLabel(scope string) string {
	switch normalizeNotifyScope(scope) {
	case "monitor":
		return "监控告警"
	case "schedule":
		return "定时任务"
	case "job":
		return "作业编排"
	case "pipeline":
		return "CI/CD 流水线"
	default:
		return "全部场景"
	}
}

func notifyEventMatch(events []string, event, status string) bool {
	if len(events) == 0 {
		return true
	}
	for _, item := range events {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "all" || value == strings.ToLower(event) || value == strings.ToLower(status) {
			return true
		}
	}
	return false
}

func notifyRuleAcceptsEvent(events []string, event NotifyEvent) bool {
	if notifyEventMatch(events, event.Event, event.Status) {
		return true
	}
	// Pre-existing schedule rules may subscribe to success instead of the
	// newly introduced recovery event.
	return normalizeNotifyScope(event.Scope) == "schedule" && event.Event == "recovered" &&
		notifyEventMatch(events, "success", "success")
}

func newNotifyDeliveryID() string {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("NTF-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("NTF-%d-%s", time.Now().UnixMilli(), strings.ToUpper(hex.EncodeToString(random)))
}

func (s *Service) initNotifyDispatcher() {
	s.notifyDispatcherOnce.Do(func() {
		s.notifyConcurrency = make(chan struct{}, 5)
		var legacyIDs []uint
		if err := s.db.Model(&model.NotifySendLog{}).Where("delivery_id = '' OR delivery_id IS NULL").Pluck("id", &legacyIDs).Error; err == nil {
			for _, id := range legacyIDs {
				_ = s.db.Model(&model.NotifySendLog{}).Where("id = ?", id).Update("delivery_id", fmt.Sprintf("NTF-LEGACY-%d", id)).Error
			}
		}
		now := time.Now()
		_ = s.db.Model(&model.NotifySendLog{}).
			Where("status = ? AND updated_at < ?", notifyStatusSending, now.Add(-2*time.Minute)).
			Updates(map[string]any{"status": notifyStatusRetrying, "next_retry_at": now, "error_text": "服务重启后恢复未完成投递"}).Error
		go func() {
			s.dispatchPendingNotifications()
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				s.dispatchPendingNotifications()
			}
		}()
	})
}

func (s *Service) dispatchPendingNotifications() {
	now := time.Now()
	var list []model.NotifySendLog
	if err := s.db.Where("status IN ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", []string{notifyStatusPending, notifyStatusRetrying}, now).
		Order("id ASC").Limit(20).Find(&list).Error; err != nil {
		return
	}
	for _, item := range list {
		claimed := s.db.Model(&model.NotifySendLog{}).
			Where("id = ? AND status IN ?", item.ID, []string{notifyStatusPending, notifyStatusRetrying}).
			Updates(map[string]any{"status": notifyStatusSending, "next_retry_at": nil})
		if claimed.Error != nil || claimed.RowsAffected == 0 {
			continue
		}
		s.notifyConcurrency <- struct{}{}
		go func(id uint) {
			defer func() { <-s.notifyConcurrency }()
			s.processNotifySendLog(id)
		}(item.ID)
	}
}

func (s *Service) processNotifySendLog(id uint) {
	var item model.NotifySendLog
	if err := s.db.First(&item, id).Error; err != nil {
		return
	}
	startedAt := time.Now()
	attempt := item.AttemptCount + 1
	result := notifyWebhookResult{}
	var sendErr error
	var channel model.NotifyChannel
	if err := s.db.First(&channel, item.ChannelID).Error; err != nil {
		sendErr = fmt.Errorf("通知媒介不存在: %w", err)
	} else if channel.Status != 1 {
		sendErr = errors.New("通知媒介已禁用")
	} else {
		result, sendErr = postNotifyWebhook(channel, []byte(item.RequestBody))
	}

	updates := map[string]any{
		"attempt_count":   attempt,
		"last_attempt_at": startedAt,
		"duration_ms":     time.Since(startedAt).Milliseconds(),
		"http_status":     result.HTTPStatus,
		"business_code":   result.BusinessCode,
		"response":        result.Body,
	}
	if sendErr == nil {
		updates["status"] = notifyStatusSuccess
		updates["error_text"] = ""
		updates["next_retry_at"] = nil
	} else {
		updates["error_text"] = sendErr.Error()
		if attempt < maxInt(item.MaxAttempts, defaultNotifyRetries) {
			nextRetryAt := time.Now().Add(notifyRetryDelay(attempt))
			updates["status"] = notifyStatusRetrying
			updates["next_retry_at"] = nextRetryAt
		} else {
			updates["status"] = notifyStatusFailed
			updates["next_retry_at"] = nil
		}
	}
	_ = s.db.Model(&model.NotifySendLog{}).Where("id = ?", id).Updates(updates).Error
}

func notifyRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 10 * time.Second
	case 2:
		return 30 * time.Second
	default:
		return 2 * time.Minute
	}
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func renderNotifyTemplate(template string, event NotifyEvent) string {
	statusColor, statusTone := notifyStatusStyle(event.Status)
	status := notifyStatusLabel(event.Scope, event.Status)
	values := map[string]string{
		"scope":       event.Scope,
		"event":       event.Event,
		"targetId":    fmt.Sprintf("%d", event.TargetID),
		"targetName":  event.TargetName,
		"status":      status,
		"summary":     event.Summary,
		"detail":      event.Detail,
		"startedAt":   formatNotifyTime(event.StartedAt),
		"finishedAt":  formatNotifyTime(event.FinishedAt),
		"statusColor": statusColor,
		"statusTone":  statusTone,
		// Alert fields have sensible fallbacks for job and schedule templates.
		// This prevents a wrongly selected template from exposing raw {{tokens}}.
		"alertName":      event.TargetName,
		"severity":       "-",
		"datasourceName": "-",
		"instance":       "-",
		"value":          "-",
		"threshold":      "-",
		"taskName":       event.TargetName,
		"taskType":       "-",
		"triggerType":    "-",
		"cronExpr":       "-",
		"duration":       "-",
		"durationMs":     "-",
		"httpStatus":     "-",
		"expectedStatus": "-",
		"jobName":        event.TargetName,
		"jobHistoryId":   "-",
		"stepName":       "-",
		"stepMessage":    event.Summary,
		"notifyAt":       formatNotifyTime(event.FinishedAt),
	}
	for key, value := range event.Extra {
		values[key] = value
	}
	return notifyTemplateVariablePattern.ReplaceAllStringFunc(template, func(token string) string {
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(token, "{{"), "}}"))
		if value, ok := values[key]; ok {
			return value
		}
		return "-"
	})
}

func notifyStatusLabel(scope, status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch normalizeNotifyScope(scope) {
	case "monitor":
		switch value {
		case "firing":
			return "触发中"
		case "recovered", "resolved":
			return "已恢复"
		case "claimed":
			return "已认领"
		}
	case "job":
		switch value {
		case "notice", "notify":
			return "通知"
		case "success", "completed":
			return "成功"
		case "failed", "error":
			return "失败"
		case "running":
			return "执行中"
		case "waiting_approval":
			return "等待人工确认"
		case "rejected":
			return "已拒绝"
		}
	case "pipeline":
		switch value {
		case "notice", "notify":
			return "通知"
		case "success", "completed":
			return "成功"
		case "failed", "error":
			return "失败"
		case "running":
			return "执行中"
		case "waiting_approval":
			return "等待人工确认"
		case "rejected":
			return "已拒绝"
		}
	case "schedule":
		return scheduleNotifyStatusLabel(status)
	}
	return status
}

func normalizeNotifyTemplateForEvent(title, content string, event NotifyEvent) (string, string) {
	scope := normalizeNotifyScope(event.Scope)
	if scope != "job" && scope != "schedule" && scope != "pipeline" {
		return title, content
	}
	monitorTokens := []string{"{{severity}}", "{{alertName}}", "{{datasourceName}}", "{{instance}}", "{{value}}", "{{threshold}}"}
	combined := title + "\n" + content
	usesMonitorFields := false
	for _, token := range monitorTokens {
		if strings.Contains(combined, token) {
			usesMonitorFields = true
			break
		}
	}
	if !usesMonitorFields {
		return title, content
	}
	if scope == "schedule" {
		return "【定时任务】{{taskName}} · {{status}}", "**执行状态：** {{status}}\n\n**任务名称：** {{taskName}}\n**任务类型：** {{taskType}}\n**触发方式：** {{triggerType}}\n**Cron：** {{cronExpr}}\n**执行耗时：** {{duration}}\n**完成时间：** {{finishedAt}}\n\n---\n\n**执行摘要**\n{{summary}}\n\n{{detail}}"
	}
	if scope == "pipeline" {
		return "【流水线通知】{{pipelineName}} · {{stageName}}", "**执行状态：** {{status}}\n\n**流水线：** {{pipelineName}}\n**执行编号：** #{{pipelineRunId}}\n**应用：** {{appName}}\n**环境：** {{env}}\n**分支：** {{branch}}\n**镜像版本：** {{imageTag}}\n**通知时间：** {{notifyAt}}\n\n---\n\n**执行摘要**\n{{summary}}\n\n{{detail}}"
	}
	return "【作业通知】{{jobName}} · {{stepName}}", "**通知类型：** {{status}}\n\n**作业名称：** {{jobName}}\n**执行编号：** #{{jobHistoryId}}\n**当前步骤：** {{stepName}}\n**触发方式：** {{triggerType}}\n**通知时间：** {{notifyAt}}\n\n---\n\n**通知摘要**\n{{summary}}\n\n{{detail}}"
}

func scheduleNotifyStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "completed":
		return "成功"
	case "failed", "error":
		return "失败"
	case "recovered":
		return "已恢复"
	case "running":
		return "执行中"
	default:
		return status
	}
}

func notifyStatusStyle(status string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "recovered", "resolved", "success", "completed":
		return "#00B42A", "info"
	case "firing", "failed", "error":
		return "#F53F3F", "warning"
	case "notice", "notify":
		return "#165DFF", "info"
	default:
		return "#FF7D00", "comment"
	}
}

func feishuHeaderTemplate(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "recovered", "resolved", "success", "completed":
		return "green"
	case "firing", "failed", "error":
		return "red"
	case "notice", "notify":
		return "blue"
	default:
		return "orange"
	}
}

func formatNotifyTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02 15:04:05")
}

func buildNotifyBody(channelType, title, content string, event NotifyEvent) ([]byte, error) {
	switch normalizeNotifyChannelType(channelType) {
	case "dingtalk":
		return json.Marshal(map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"title": title, "text": content},
		})
	case "wecom":
		return json.Marshal(map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"content": content},
		})
	case "feishu":
		return json.Marshal(map[string]any{
			"msg_type": "interactive",
			"card": map[string]any{
				"header": map[string]any{
					"template": feishuHeaderTemplate(event.Status),
					"title":    map[string]string{"tag": "plain_text", "content": title},
				},
				"elements": []map[string]any{{"tag": "markdown", "content": content}},
			},
		})
	default:
		return json.Marshal(map[string]any{
			"title": title, "content": content,
			"scope": event.Scope, "event": event.Event, "status": event.Status,
			"targetId": event.TargetID, "targetName": event.TargetName,
			"summary": event.Summary, "detail": event.Detail,
		})
	}
}

type notifyWebhookResult struct {
	Body         string
	HTTPStatus   int
	BusinessCode string
}

func postNotifyWebhook(channel model.NotifyChannel, body []byte) (notifyWebhookResult, error) {
	result := notifyWebhookResult{}
	webhookURL := strings.TrimSpace(channel.WebhookURL)
	if webhookURL == "" {
		return result, errors.New("webhook 地址为空")
	}
	if channel.Secret != "" && normalizeNotifyChannelType(channel.ChannelType) == "dingtalk" {
		signedURL, err := signDingTalkURL(webhookURL, channel.Secret)
		if err != nil {
			return result, err
		}
		webhookURL = signedURL
	}
	request, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range parseHeaderMap(channel.HeadersJSON) {
		request.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 32768))
	result.Body = string(responseBody)
	result.HTTPStatus = response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("Webhook 返回 HTTP %d", response.StatusCode)
	}
	code, message, exists := parseNotifyBusinessResponse(channel.ChannelType, responseBody)
	result.BusinessCode = code
	if exists && code != "0" {
		return result, fmt.Errorf("平台返回业务错误码 %s: %s", code, firstNonEmpty(message, "未知错误"))
	}
	return result, nil
}

func parseNotifyBusinessResponse(channelType string, body []byte) (string, string, bool) {
	var payload map[string]any
	if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, &payload) != nil {
		return "", "", false
	}
	var codeKey string
	switch normalizeNotifyChannelType(channelType) {
	case "dingtalk", "wecom":
		codeKey = "errcode"
	case "feishu":
		codeKey = "code"
	default:
		return "", "", false
	}
	value, exists := payload[codeKey]
	if !exists {
		return "", "", false
	}
	code := notifyBusinessCode(value)
	message := notifyResponseMessage(payload, "errmsg", "msg", "message")
	return code, message, true
}

func notifyResponseMessage(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, exists := payload[key]; exists && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func notifyBusinessCode(value any) string {
	switch code := value.(type) {
	case float64:
		return strconv.FormatInt(int64(code), 10)
	case json.Number:
		return code.String()
	case string:
		return strings.TrimSpace(code)
	default:
		return strings.TrimSpace(fmt.Sprint(code))
	}
}

func signDingTalkURL(rawURL, secret string) (string, error) {
	timestamp := time.Now().UnixMilli()
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(stringToSign))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(h.Sum(nil)))
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%stimestamp=%d&sign=%s", rawURL, sep, timestamp, sign), nil
}

func (s *Service) ListNotifySendLogs(pageNum, pageSize int, keyword, status, channelType, scope, startTime, endTime string) (map[string]any, error) {
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	query := s.db.Model(&model.NotifySendLog{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("rule_name LIKE ? OR channel_name LIKE ? OR target_name LIKE ? OR summary LIKE ?", like, like, like, like)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(channelType) != "" {
		query = query.Where("channel_type = ?", normalizeNotifyChannelType(channelType))
	}
	if strings.TrimSpace(scope) != "" {
		query = query.Where("scope = ?", normalizeNotifyScope(scope))
	}
	if value, ok := parseNotifyQueryTime(startTime); ok {
		query = query.Where("created_at >= ?", value)
	}
	if value, ok := parseNotifyQueryTime(endTime); ok {
		query = query.Where("created_at <= ?", value)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var list []model.NotifySendLog
	if err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return map[string]any{"list": list, "total": total, "pageNum": pageNum, "pageSize": pageSize}, nil
}

func parseNotifyQueryTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (s *Service) RetryNotifySendLog(id uint) (map[string]any, error) {
	var original model.NotifySendLog
	if err := s.db.First(&original, id).Error; err != nil {
		return nil, err
	}
	if original.Status != notifyStatusFailed {
		return nil, errors.New("仅失败的投递记录可以重新发送")
	}
	now := time.Now()
	retry := model.NotifySendLog{
		DeliveryID: newNotifyDeliveryID(),
		RuleID:     original.RuleID, RuleName: original.RuleName,
		ChannelID: original.ChannelID, ChannelName: original.ChannelName, ChannelType: original.ChannelType,
		Event: original.Event, Scope: original.Scope,
		TargetID: original.TargetID, TargetName: original.TargetName, Summary: original.Summary,
		Status: notifyStatusPending, MaxAttempts: defaultNotifyRetries, NextRetryAt: &now,
		RetryOfID: original.ID, RequestBody: original.RequestBody,
	}
	if err := s.db.Create(&retry).Error; err != nil {
		return nil, err
	}
	return map[string]any{"id": retry.ID, "deliveryId": retry.DeliveryID}, nil
}
