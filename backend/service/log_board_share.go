package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"ops-admin/backend/model"
)

// 公开链接的状态需要区分「不存在」「被停用 / 过期」「访问过频」，
// 前端才能给出准确提示，而不是统一报一个 404。
var (
	ErrLogBoardShareNotFound = errors.New("分享链接不存在或已被删除")
	ErrLogBoardShareDisabled = errors.New("分享链接已停用")
	ErrLogBoardShareExpired  = errors.New("分享链接已过期")
	ErrLogBoardShareTooBusy  = errors.New("当前访问过于频繁，请稍后再试")
)

var logBoardShareTokenPattern = regexp.MustCompile(`^[a-f0-9]{32,64}$`)

const (
	logBoardShareDefaultRange   = 24 * 3600
	logBoardShareMinRange       = 300
	logBoardShareMaxRange       = 7 * 24 * 3600
	logBoardShareDefaultRefresh = 60
	logBoardShareMinRefresh     = 15
	logBoardShareMaxRefresh     = 600
	logBoardShareMaxQueryLen    = 2000
	logBoardShareSampleLimit    = 5000
	logBoardShareRateWindow     = time.Minute
	logBoardShareRateBurst      = 40
)

type LogBoardSharePayload struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	DatasourceID     uint   `json:"datasourceId"`
	Query            string `json:"query"`
	RangeSeconds     int    `json:"rangeSeconds"`
	RefreshSeconds   int    `json:"refreshSeconds"`
	AllowDetail      *bool  `json:"allowDetail"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
	Status           *int   `json:"status"`
}

// PublicLogBoardQuery 是公开页传给服务端的查询参数。数据源、LogsQL 和时间范围
// 都允许查看者自己改，但只能在服务端允许的范围内使用。
type PublicLogBoardQuery struct {
	DatasourceID uint   `json:"datasourceId"`
	Query        string `json:"query"`
	RangeSeconds int    `json:"rangeSeconds"`
}

// LogBoardShareView 是管理端看到的分享信息：带数据源名称和当前是否可用。
type LogBoardShareView struct {
	model.LogBoardShare
	DatasourceName string `json:"datasourceName"`
	Active         bool   `json:"active"`
	Unavailable    string `json:"unavailable"`
}

func (s *Service) ListLogBoardShares() ([]LogBoardShareView, error) {
	var shares []model.LogBoardShare
	if err := s.db.Order("id DESC").Find(&shares).Error; err != nil {
		return nil, err
	}
	names := map[uint]string{}
	for _, share := range shares {
		if _, ok := names[share.DatasourceID]; ok {
			continue
		}
		var ds model.MonitorDatasource
		if err := s.db.Select("id", "name").First(&ds, share.DatasourceID).Error; err == nil {
			names[share.DatasourceID] = ds.Name
		}
	}
	now := time.Now()
	result := make([]LogBoardShareView, 0, len(shares))
	for _, share := range shares {
		result = append(result, logBoardShareView(share, names[share.DatasourceID], now))
	}
	return result, nil
}

func (s *Service) SaveLogBoardShare(payload LogBoardSharePayload) (LogBoardShareView, error) {
	name := strings.TrimSpace(payload.Name)
	description := strings.TrimSpace(payload.Description)
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		query = "*"
	}
	if utf8.RuneCountInString(name) > 80 {
		return LogBoardShareView{}, errors.New("大屏名称不能超过 80 个字符")
	}
	if utf8.RuneCountInString(description) > 200 {
		return LogBoardShareView{}, errors.New("大屏说明不能超过 200 个字符")
	}
	if name == "" {
		name = "日志大屏 · " + query
		if utf8.RuneCountInString(name) > 80 {
			name = string([]rune(name)[:80])
		}
	}
	if payload.DatasourceID == 0 {
		return LogBoardShareView{}, errors.New("请选择日志数据源")
	}
	if err := s.requireResource("datasource", payload.DatasourceID, "", "query"); err != nil {
		return LogBoardShareView{}, err
	}
	ds, err := s.GetMonitorDatasource(payload.DatasourceID)
	if err != nil {
		return LogBoardShareView{}, errors.New("日志数据源不存在")
	}
	if normalizeMonitorDatasourceType(ds.Type) != "victorialogs" {
		return LogBoardShareView{}, errors.New("日志大屏当前仅支持 VictoriaLogs 数据源")
	}
	rangeSeconds := payload.RangeSeconds
	if rangeSeconds <= 0 {
		rangeSeconds = logBoardShareDefaultRange
	}
	if rangeSeconds < logBoardShareMinRange || rangeSeconds > logBoardShareMaxRange {
		return LogBoardShareView{}, errors.New("时间范围需在 5 分钟到 7 天之间")
	}
	refreshSeconds := payload.RefreshSeconds
	if refreshSeconds <= 0 {
		refreshSeconds = logBoardShareDefaultRefresh
	}
	if refreshSeconds < logBoardShareMinRefresh || refreshSeconds > logBoardShareMaxRefresh {
		return LogBoardShareView{}, errors.New("刷新频率需在 15 秒到 10 分钟之间")
	}
	allowDetail := true
	if payload.AllowDetail != nil {
		allowDetail = *payload.AllowDetail
	}

	var share model.LogBoardShare
	if payload.ID > 0 {
		if err := s.db.First(&share, payload.ID).Error; err != nil {
			return LogBoardShareView{}, errors.New("分享链接不存在")
		}
		// 修改分享时，也要确认当前用户对原数据源仍有查询权限。
		if err := s.requireResource("datasource", share.DatasourceID, "", "query"); err != nil {
			return LogBoardShareView{}, err
		}
	} else {
		token, err := newLogBoardShareToken()
		if err != nil {
			return LogBoardShareView{}, err
		}
		share.Token = token
		share.Status = 1
		share.CreatedBy = currentLogBoardShareActor(s)
	}

	share.Name = name
	share.Description = description
	share.DatasourceID = payload.DatasourceID
	share.Query = query
	share.RangeSeconds = rangeSeconds
	share.RefreshSeconds = refreshSeconds
	share.AllowDetail = allowDetail
	if payload.Status != nil {
		share.Status = normalizeLogBoardShareStatus(*payload.Status)
	}
	if payload.ExpiresInSeconds > 0 {
		expiresAt := time.Now().Add(time.Duration(payload.ExpiresInSeconds) * time.Second)
		share.ExpiresAt = &expiresAt
	} else {
		share.ExpiresAt = nil
	}

	if err := s.db.Save(&share).Error; err != nil {
		return LogBoardShareView{}, err
	}
	return logBoardShareView(share, ds.Name, time.Now()), nil
}

func (s *Service) SetLogBoardShareStatus(id uint, status int) (LogBoardShareView, error) {
	var share model.LogBoardShare
	if err := s.db.First(&share, id).Error; err != nil {
		return LogBoardShareView{}, errors.New("分享链接不存在")
	}
	share.Status = normalizeLogBoardShareStatus(status)
	if err := s.db.Model(&share).Update("status", share.Status).Error; err != nil {
		return LogBoardShareView{}, err
	}
	var ds model.MonitorDatasource
	_ = s.db.Select("id", "name").First(&ds, share.DatasourceID).Error
	return logBoardShareView(share, ds.Name, time.Now()), nil
}

func (s *Service) RegenerateLogBoardShareToken(id uint) (LogBoardShareView, error) {
	var share model.LogBoardShare
	if err := s.db.First(&share, id).Error; err != nil {
		return LogBoardShareView{}, errors.New("分享链接不存在")
	}
	token, err := newLogBoardShareToken()
	if err != nil {
		return LogBoardShareView{}, err
	}
	share.Token = token
	if err := s.db.Model(&share).Update("token", token).Error; err != nil {
		return LogBoardShareView{}, err
	}
	var ds model.MonitorDatasource
	_ = s.db.Select("id", "name").First(&ds, share.DatasourceID).Error
	return logBoardShareView(share, ds.Name, time.Now()), nil
}

func (s *Service) DeleteLogBoardShare(id uint) error {
	result := s.db.Delete(&model.LogBoardShare{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("分享链接不存在")
	}
	return nil
}

// GetPublicLogBoardMeta 只返回展示与交互需要的元信息，不触发日志查询。
// 页面打开时计数一次，自动刷新不会重复累加。
func (s *Service) GetPublicLogBoardMeta(token string) (map[string]any, error) {
	share, err := s.findPublicLogBoardShare(token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	_ = s.db.Model(&model.LogBoardShare{}).Where("id = ?", share.ID).Updates(map[string]any{
		"view_count":     gorm.Expr("view_count + 1"),
		"last_viewed_at": now,
	}).Error
	return s.logBoardShareMetaPayload(share)
}

// GetPublicLogBoardData 按查看者当前的交互参数执行分析。数据源限定为
// VictoriaLogs，时间范围与查询长度都夹在上限内，取样条数固定 5000。
func (s *Service) GetPublicLogBoardData(token string, query PublicLogBoardQuery, clientIP string) (map[string]any, error) {
	share, err := s.findPublicLogBoardShare(token)
	if err != nil {
		return nil, err
	}
	if !logBoardShareRateLimiter.allow(logBoardShareRateKey(share.Token, clientIP)) {
		return nil, ErrLogBoardShareTooBusy
	}

	datasourceID := query.DatasourceID
	if datasourceID == 0 {
		datasourceID = share.DatasourceID
	}
	ds, err := s.GetMonitorDatasource(datasourceID)
	if err != nil {
		return nil, errors.New("日志数据源不存在或已删除")
	}
	if normalizeMonitorDatasourceType(ds.Type) != "victorialogs" {
		return nil, errors.New("日志大屏当前仅支持 VictoriaLogs 数据源")
	}

	logsQL := strings.TrimSpace(query.Query)
	if logsQL == "" {
		logsQL = strings.TrimSpace(share.Query)
	}
	if logsQL == "" {
		logsQL = "*"
	}
	if utf8.RuneCountInString(logsQL) > logBoardShareMaxQueryLen {
		return nil, errors.New("过滤条件过长，请精简后再查询")
	}

	rangeSeconds := query.RangeSeconds
	if rangeSeconds <= 0 {
		rangeSeconds = share.RangeSeconds
	}
	if rangeSeconds < logBoardShareMinRange {
		rangeSeconds = logBoardShareMinRange
	}
	if rangeSeconds > logBoardShareMaxRange {
		rangeSeconds = logBoardShareMaxRange
	}

	now := time.Now()
	_ = s.db.Model(&model.LogBoardShare{}).Where("id = ?", share.ID).Update("last_viewed_at", now).Error

	cacheKey := logBoardShareCacheKey(share.Token, datasourceID, logsQL, rangeSeconds)
	if cached, ok := logBoardShareCache.get(cacheKey); ok {
		return cached, nil
	}

	analysis, err := s.AnalyzeMonitorLogBoard(MonitorLogBoardPayload{
		DatasourceID: datasourceID,
		Query:        logsQL,
		StartAt:      now.Add(-time.Duration(rangeSeconds) * time.Second).UnixMilli(),
		EndAt:        now.UnixMilli(),
		Limit:        logBoardShareSampleLimit,
	})
	if err != nil {
		return nil, err
	}
	if !share.AllowDetail {
		stripLogBoardShareDetail(analysis)
	}
	meta, err := s.logBoardShareMetaPayload(share)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"meta":         meta,
		"analysis":     analysis,
		"refreshedAt":  now.UnixMilli(),
		"rangeSeconds": rangeSeconds,
		"serverTime":   now.Format("2006-01-02 15:04:05"),
	}
	logBoardShareCache.set(cacheKey, result, logBoardShareCacheTTL(share.RefreshSeconds))
	return result, nil
}

func (s *Service) findPublicLogBoardShare(token string) (model.LogBoardShare, error) {
	token = strings.ToLower(strings.TrimSpace(token))
	if !logBoardShareTokenPattern.MatchString(token) {
		return model.LogBoardShare{}, ErrLogBoardShareNotFound
	}
	var share model.LogBoardShare
	if err := s.db.Where("token = ?", token).First(&share).Error; err != nil {
		return model.LogBoardShare{}, ErrLogBoardShareNotFound
	}
	now := time.Now()
	if share.Status != 1 {
		return model.LogBoardShare{}, ErrLogBoardShareDisabled
	}
	if share.ExpiresAt != nil && now.After(*share.ExpiresAt) {
		return model.LogBoardShare{}, ErrLogBoardShareExpired
	}
	return share, nil
}

// logBoardShareMetaPayload 返回大屏标题、默认查询以及可切换的数据源清单。
// 清单只包含名称和 ID，绝不带出连接地址与凭据。
func (s *Service) logBoardShareMetaPayload(share model.LogBoardShare) (map[string]any, error) {
	datasources, err := s.ListMonitorDatasourceOptions()
	if err != nil {
		return nil, err
	}
	options := make([]map[string]any, 0, len(datasources))
	for _, item := range datasources {
		if normalizeMonitorDatasourceType(item.Type) != "victorialogs" {
			continue
		}
		options = append(options, map[string]any{"id": item.ID, "name": item.Name})
	}
	if len(options) == 0 {
		return nil, errors.New("暂无可用的 VictoriaLogs 数据源")
	}
	meta := map[string]any{
		"token":           share.Token,
		"name":            share.Name,
		"description":     share.Description,
		"query":           share.Query,
		"datasourceId":    share.DatasourceID,
		"rangeSeconds":    share.RangeSeconds,
		"refreshSeconds":  share.RefreshSeconds,
		"allowDetail":     share.AllowDetail,
		"datasources":     options,
		"maxRangeSeconds": logBoardShareMaxRange,
		"maxQueryLength":  logBoardShareMaxQueryLen,
	}
	if share.ExpiresAt != nil {
		meta["expiresAt"] = share.ExpiresAt.UnixMilli()
	}
	return meta, nil
}

func logBoardShareView(share model.LogBoardShare, datasourceName string, now time.Time) LogBoardShareView {
	view := LogBoardShareView{
		LogBoardShare:  share,
		DatasourceName: datasourceName,
		Active:         true,
	}
	if share.Status != 1 {
		view.Active = false
		view.Unavailable = "已停用"
	} else if share.ExpiresAt != nil && now.After(*share.ExpiresAt) {
		view.Active = false
		view.Unavailable = "已过期"
	}
	return view
}

func normalizeLogBoardShareStatus(value int) int {
	if value == 0 {
		return 0
	}
	return 1
}

func newLogBoardShareToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

// currentLogBoardShareActor 记录分享创建人，便于审计谁把日志公开出去了。
func currentLogBoardShareActor(s *Service) string {
	if policy := s.resourcePolicy(); policy != nil && policy.Username != "" {
		return policy.Username
	}
	return ""
}

// stripLogBoardShareDetail 在「不允许查看详情」时移除日志正文，
// 只保留首行摘要，避免堆栈里的参数和内部标识泄露。
func stripLogBoardShareDetail(analysis map[string]any) {
	if analysis == nil {
		return
	}
	recent, _ := analysis["recent"].([]map[string]any)
	for _, item := range recent {
		delete(item, "message")
	}
	for _, key := range []string{"pods", "containers"} {
		delete(analysis, key)
	}
}

func logBoardShareCacheTTL(refreshSeconds int) time.Duration {
	ttl := time.Duration(refreshSeconds) * time.Second
	if ttl > 8*time.Second {
		ttl = 8 * time.Second
	}
	if ttl < 3*time.Second {
		ttl = 3 * time.Second
	}
	return ttl
}

func logBoardShareCacheKey(token string, datasourceID uint, query string, rangeSeconds int) string {
	// 时间窗参与 key：不同查看者同一分钟内的同一查询共用一个结果，
	// 换数据源、改条件、切范围都会自然击穿到新查询。
	return strings.Join([]string{
		token,
		strconv.FormatUint(uint64(datasourceID), 10),
		strconv.Itoa(rangeSeconds),
		query,
		time.Now().Format("200601021504"),
	}, "|")
}

type logBoardShareCacheEntry struct {
	value     map[string]any
	expiresAt time.Time
}

// logBoardShareCache 让同一分钟内的多个浏览者共用一次日志查询；
// 客户端离线刷新、投屏轮询都不会把 VictoriaLogs 打满。
var logBoardShareCache = &logBoardShareCacheStore{entries: map[string]logBoardShareCacheEntry{}}

type logBoardShareCacheStore struct {
	mu      sync.Mutex
	entries map[string]logBoardShareCacheEntry
}

func (c *logBoardShareCacheStore) get(key string) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(c.entries, key)
		}
		return nil, false
	}
	return entry.value, true
}

func (c *logBoardShareCacheStore) set(key string, value map[string]any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for existing, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, existing)
		}
	}
	c.entries[key] = logBoardShareCacheEntry{value: value, expiresAt: now.Add(ttl)}
}

func logBoardShareRateKey(token, clientIP string) string {
	return token + "|" + strings.TrimSpace(clientIP)
}

// logBoardShareRateLimiterStore 是公开链接的粗粒度限流：同一 token + IP 每分钟最多
// 40 次分析。公开页允许改条件，没有限流就等于把日志集群直接暴露出去。
type logBoardShareRateLimiterStore struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *logBoardShareRateLimiterStore) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.recentLocked(key, now)
	if len(recent) >= logBoardShareRateBurst {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func (l *logBoardShareRateLimiterStore) recentLocked(key string, now time.Time) []time.Time {
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	cutoff := now.Add(-logBoardShareRateWindow)
	source := l.hits[key]
	kept := make([]time.Time, 0, len(source))
	for _, at := range source {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	return kept
}

var logBoardShareRateLimiter = &logBoardShareRateLimiterStore{hits: map[string][]time.Time{}}
