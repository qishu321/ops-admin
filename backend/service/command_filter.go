package service

import (
	"errors"
	"fmt"
	"ops-admin/backend/internal/access"
	"ops-admin/backend/model"
	"regexp"
	"slices"
	"strings"
)

func (s *Service) commandFilterAdmin() error {
	if p := s.resourcePolicy(); p != nil && !p.Super {
		return access.ErrDenied
	}
	return nil
}
func (s *Service) ListCommandFilters() ([]model.CommandFilter, error) {
	if err := s.commandFilterAdmin(); err != nil {
		return nil, err
	}
	rows := []model.CommandFilter{}
	err := s.db.Order("id DESC").Find(&rows).Error
	return rows, err
}
func validateCommandFilter(rule *model.CommandFilter) error {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Pattern = strings.TrimSpace(rule.Pattern)
	rule.Description = strings.TrimSpace(rule.Description)
	if rule.Name == "" || len(rule.Name) > 128 || rule.Pattern == "" || len(rule.Pattern) > 4096 || len(rule.Description) > 500 {
		return errors.New("请填写规则名称和匹配内容，内容最多 4096 字节")
	}
	if rule.MatchType != "keyword" && rule.MatchType != "regex" {
		return errors.New("匹配方式不正确")
	}
	if rule.MatchType == "keyword" && len(commandFilterKeywords(rule.Pattern)) == 0 {
		return errors.New("请填写至少一个有效关键词，使用换行、逗号或顿号分隔")
	}
	if rule.MatchType == "regex" {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return errors.New("正则表达式不正确: " + err.Error())
		}
	}
	if len(rule.Scopes) == 0 {
		return errors.New("至少选择终端或作业一个适用范围")
	}
	for _, scope := range rule.Scopes {
		if scope != "terminal" && scope != "job" {
			return errors.New("适用范围不正确")
		}
	}
	rule.Scopes = slices.Compact(slices.Sorted(slices.Values(rule.Scopes)))
	return nil
}
func (s *Service) SaveCommandFilter(rule model.CommandFilter) error {
	if err := s.commandFilterAdmin(); err != nil {
		return err
	}
	if err := validateCommandFilter(&rule); err != nil {
		return err
	}
	if rule.ID > 0 {
		var old model.CommandFilter
		if err := s.db.First(&old, rule.ID).Error; err != nil {
			return err
		}
		rule.CreatedAt = old.CreatedAt
	}
	return s.db.Save(&rule).Error
}
func (s *Service) DeleteCommandFilter(id uint) error {
	if err := s.commandFilterAdmin(); err != nil {
		return err
	}
	if id == 0 {
		return errors.New("请选择规则")
	}
	return s.db.Delete(&model.CommandFilter{}, id).Error
}

func commandFilterKeywords(pattern string) []string {
	parts := strings.FieldsFunc(pattern, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '\n' || r == '\r'
	})
	keywords := []string{}
	for _, part := range parts {
		keyword := strings.ToLower(strings.Join(strings.Fields(part), " "))
		if keyword != "" && !slices.Contains(keywords, keyword) {
			keywords = append(keywords, keyword)
		}
	}
	return keywords
}

// Keywords match independently (OR), ignore case and normalize whitespace. Regular expressions use Go's
// bounded RE2 engine. Only enabled rules in the requested scope participate.
func matchCommandFilters(rules []model.CommandFilter, scope, command string) error {
	normalized := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(command, "\\\n", "")), " "))
	for _, rule := range rules {
		if !rule.Enabled || !slices.Contains(rule.Scopes, scope) {
			continue
		}
		matched := false
		if rule.MatchType == "keyword" {
			for _, keyword := range commandFilterKeywords(rule.Pattern) {
				if strings.Contains(normalized, keyword) {
					matched = true
					break
				}
			}
		} else if rule.MatchType == "regex" {
			r, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return errors.New("命令过滤规则配置异常，请联系管理员")
			}
			matched = r.MatchString(command)
		}
		if matched {
			return fmt.Errorf("命令已被过滤规则“%s”拦截", rule.Name)
		}
	}
	return nil
}
func (s *Service) enabledCommandFilters() ([]model.CommandFilter, error) {
	var rules []model.CommandFilter
	if err := s.db.Where("enabled=?", true).Order("id").Find(&rules).Error; err != nil {
		return nil, errors.New("无法读取命令过滤配置，已停止执行")
	}
	return rules, nil
}
func (s *Service) CheckCommandFilters(scope, command string) error {
	rules, err := s.enabledCommandFilters()
	if err != nil {
		return err
	}
	return matchCommandFilters(rules, scope, command)
}
func jobFilterContent(content, params string, variables map[string]string) string {
	values := []string{content, params}
	for _, key := range slices.Sorted(slices.Values(mapKeys(variables))) {
		values = append(values, variables[key])
	}
	return strings.Join(values, "\n")
}
func mapKeys(values map[string]string) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
