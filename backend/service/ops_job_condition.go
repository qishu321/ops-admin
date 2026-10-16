package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"ops-admin/backend/model"

	"gorm.io/gorm"
)

// Branches are exclusive choices. Ordinary fan-out remains an execution
// dependency, and a join runs once after all incoming paths have been resolved.
func validateOpsJobBranches(definition OpsJobDefinition) error {
	nodes := make(map[string]OpsJobNode, len(definition.Nodes))
	incoming := map[string][]OpsJobEdge{}
	outgoing := map[string][]OpsJobEdge{}
	seenEdges := map[OpsJobEdge]bool{}
	for _, node := range definition.Nodes {
		if node.ID == "" || nodes[node.ID].ID != "" {
			return errors.New("步骤 ID 不能为空或重复")
		}
		nodes[node.ID] = node
	}
	for _, edge := range definition.Edges {
		if nodes[edge.Source].ID == "" || nodes[edge.Target].ID == "" || edge.Source == edge.Target || seenEdges[edge] {
			return errors.New("存在无效、重复或连接到自身的连线")
		}
		seenEdges[edge] = true
		incoming[edge.Target] = append(incoming[edge.Target], edge)
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		if nodes[edge.Source].Type == "condition" {
			if edge.Branch != "true" && edge.Branch != "false" {
				return errors.New("条件分支的连线必须指定满足条件或不满足路径")
			}
		} else if edge.Branch != "" {
			return errors.New("只有条件分支可以设置分支路径")
		}
	}
	for _, node := range definition.Nodes {
		policy := stringConfig(node.Config, "failurePolicy")
		if policy != "" && policy != "stop" && policy != "branch" {
			return fmt.Errorf("步骤“%s”的失败处理方式不正确", node.Label)
		}
		if policy == "branch" {
			if (node.Type != "script" && node.Type != "file") || len(outgoing[node.ID]) == 0 {
				return errors.New("失败后进入分支仅支持直接连接条件分支的脚本或文件步骤")
			}
			for _, edge := range outgoing[node.ID] {
				target := nodes[edge.Target]
				if target.Type != "condition" || stringConfig(target.Config, "sourceStepId") != node.ID {
					return fmt.Errorf("步骤“%s”设置失败后进入分支时，所有后续步骤必须是判断此步骤的条件分支", node.Label)
				}
			}
		}
		if node.Type != "condition" {
			continue
		}
		inputs := incoming[node.ID]
		if len(inputs) != 1 {
			return fmt.Errorf("条件“%s”必须连接且只连接一个前置步骤", node.Label)
		}
		source := nodes[inputs[0].Source]
		if (source.Type != "script" && source.Type != "file") || stringConfig(node.Config, "sourceStepId") != source.ID {
			return fmt.Errorf("条件“%s”必须判断直接连接的脚本或文件步骤", node.Label)
		}
		paths := map[string]int{}
		for _, edge := range outgoing[node.ID] {
			paths[edge.Branch]++
		}
		if paths["true"] != 1 || paths["false"] != 1 {
			return fmt.Errorf("条件“%s”的满足条件和不满足出口必须各连接一个后续步骤", node.Label)
		}
		if err := validateOpsJobCondition(node.Config); err != nil {
			return fmt.Errorf("条件“%s”：%w", node.Label, err)
		}
	}
	_, err := buildOpsJobNodeOrder(definition)
	return err
}

func jobConditionNumber(value any) (float64, error) {
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	case json.Number:
		var err error
		n, err = v.Float64()
		if err != nil {
			return 0, errors.New("比较值必须是非负整数")
		}
	default:
		return 0, errors.New("比较值必须是非负整数")
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || math.Trunc(n) != n || n > 1<<31-1 {
		return 0, errors.New("比较值必须是有效的非负整数")
	}
	return n, nil
}

func validateOpsJobCondition(config map[string]any) error {
	field, operator := stringConfig(config, "field"), stringConfig(config, "operator")
	if field == "status" {
		if operator != "eq" && operator != "ne" {
			return errors.New("执行状态只支持等于或不等于")
		}
		value := stringConfig(config, "value")
		if value != "success" && value != "partial" && value != "failed" {
			return errors.New("请选择成功、部分失败或全部失败")
		}
		return nil
	}
	if field != "successCount" && field != "failedCount" {
		return errors.New("不支持此判断字段")
	}
	if operator != "eq" && operator != "ne" && operator != "gt" && operator != "gte" && operator != "lt" && operator != "lte" {
		return errors.New("不支持此比较方式")
	}
	_, err := jobConditionNumber(config["value"])
	return err
}

func compareOpsJobCondition(config map[string]any, status string, successCount, failedCount int) (bool, string, error) {
	if err := validateOpsJobCondition(config); err != nil {
		return false, "", err
	}
	operator := stringConfig(config, "operator")
	operatorLabel := map[string]string{"eq": "等于", "ne": "不等于", "gt": "大于", "gte": "大于或等于", "lt": "小于", "lte": "小于或等于"}[operator]
	if stringConfig(config, "field") == "status" {
		value := stringConfig(config, "value")
		result := status == value
		if operator == "ne" {
			result = !result
		}
		labels := map[string]string{"success": "全部成功", "partial": "部分失败", "failed": "全部失败"}
		return result, fmt.Sprintf("执行状态：%s；比较：%s %s", labels[status], operatorLabel, labels[value]), nil
	}
	value, _ := jobConditionNumber(config["value"])
	actual, label := float64(successCount), "成功主机数"
	if stringConfig(config, "field") == "failedCount" {
		actual, label = float64(failedCount), "失败主机数"
	}
	result := false
	switch operator {
	case "eq":
		result = actual == value
	case "ne":
		result = actual != value
	case "gt":
		result = actual > value
	case "gte":
		result = actual >= value
	case "lt":
		result = actual < value
	case "lte":
		result = actual <= value
	}
	return result, fmt.Sprintf("%s：%.0f；比较：%s %.0f", label, actual, operatorLabel, value), nil
}

func (s *Service) evaluateOpsJobCondition(historyID uint, node OpsJobNode) (bool, string, error) {
	var source model.OpsJobHistoryStep
	if err := s.db.Where("history_id = ? AND step_id = ?", historyID, stringConfig(node.Config, "sourceStepId")).Order("id DESC").First(&source).Error; err != nil {
		return false, "", errors.New("找不到前置步骤的执行结果")
	}
	if source.Status != "success" && source.Status != "partial" && source.Status != "failed" {
		return false, "", errors.New("前置步骤未执行完成，无法判断条件")
	}
	var task model.OpsExecTask
	if stringConfig(node.Config, "field") != "status" {
		if source.ExecTaskID == 0 {
			return false, "", errors.New("前置步骤未产生主机执行统计，请改为判断执行状态")
		}
		if err := s.db.First(&task, source.ExecTaskID).Error; err != nil {
			return false, "", errors.New("读取前置步骤的主机执行统计失败")
		}
	}
	result, detail, err := compareOpsJobCondition(node.Config, source.Status, task.SuccessCount, task.FailedCount)
	return result, fmt.Sprintf("判断步骤：%s\n%s\n判断结果：%s", source.StepName, detail, map[bool]string{true: "满足条件", false: "不满足"}[result]), err
}

type opsJobNodeExecutor func(OpsJobNode, bool) (model.OpsJobHistoryStep, error)

// states is the persisted execution snapshot, including branch choices. Walking
// it again after approval never re-evaluates a condition or repeats a side effect.
func walkOpsJobDefinition(definition OpsJobDefinition, states map[string]model.OpsJobHistoryStep, execute opsJobNodeExecutor) (paused, handledFailure bool, err error) {
	if err = validateOpsJobBranches(definition); err != nil {
		return
	}
	order, _ := buildOpsJobNodeOrder(definition)
	incoming := map[string][]OpsJobEdge{}
	for _, edge := range definition.Edges {
		incoming[edge.Target] = append(incoming[edge.Target], edge)
	}
	for _, node := range order {
		step, exists := states[node.ID]
		if !exists {
			active := len(incoming[node.ID]) == 0
			for _, edge := range incoming[node.ID] {
				previous := states[edge.Source]
				if previous.Status == "skipped" {
					continue
				}
				if edge.Branch != "" {
					if previous.BranchResult == nil {
						return false, handledFailure, errors.New("条件判断结果缺失，已停止执行")
					}
					if strconv.FormatBool(*previous.BranchResult) != edge.Branch {
						continue
					}
				}
				active = true
			}
			step, err = execute(node, !active)
			if err != nil {
				return false, handledFailure, err
			}
			states[node.ID] = step
		}
		switch step.Status {
		case "waiting_approval":
			return true, handledFailure, nil
		case "failed":
			if stringConfig(node.Config, "failurePolicy") != "branch" {
				return false, handledFailure, fmt.Errorf("步骤“%s”执行失败：%s", node.Label, step.Summary)
			}
			handledFailure = true
		case "partial":
			handledFailure = handledFailure || stringConfig(node.Config, "failurePolicy") == "branch"
		case "success", "skipped":
		default:
			return false, handledFailure, fmt.Errorf("步骤“%s”尚未完成，不能重复执行", node.Label)
		}
	}
	return false, handledFailure, nil
}

func (s *Service) continueOpsJobDefinition(historyID uint, definition OpsJobDefinition) {
	var previous []model.OpsJobHistoryStep
	if err := s.db.Where("history_id = ?", historyID).Order("id ASC").Find(&previous).Error; err != nil {
		s.finishOpsJobHistory(historyID, "failed", "读取执行快照失败", "", "")
		return
	}
	states := map[string]model.OpsJobHistoryStep{}
	for _, step := range previous {
		states[step.StepID] = step
	}
	paused, handled, err := walkOpsJobDefinition(definition, states, func(node OpsJobNode, skipped bool) (model.OpsJobHistoryStep, error) {
		if skipped {
			now := time.Now()
			step := model.OpsJobHistoryStep{HistoryID: historyID, StepID: node.ID, StepName: node.Label, StepType: node.Type, Status: "skipped", Summary: "未选中此分支，步骤已跳过", FinishedAt: &now}
			return step, s.db.Create(&step).Error
		}
		s.runSingleOpsJobNode(historyID, node)
		var step model.OpsJobHistoryStep
		err := s.db.Where("history_id = ? AND step_id = ?", historyID, node.ID).Order("id DESC").First(&step).Error
		return step, err
	})
	if err != nil {
		finishedAt := time.Now()
		_ = s.db.Model(&model.OpsJobHistory{}).Where("id = ?", historyID).Updates(map[string]any{"status": "failed", "summary": err.Error(), "finished_at": &finishedAt}).Error
	} else if !paused {
		if handled {
			s.finishOpsJobHistory(historyID, "partial", "作业执行完成，失败步骤已进入条件分支处理", "", "")
		} else {
			s.finishOpsJobHistory(historyID, "success", "作业执行完成", "", "")
		}
	}
}

// Claim the waiting history and decide its current approval atomically. A
// duplicate click (or approval racing rejection) cannot start another worker.
func (s *Service) decideOpsJobApproval(payload OpsJobHistoryApprovalPayload, approved bool) (*OpsJobDefinition, error) {
	var definition *OpsJobDefinition
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var history model.OpsJobHistory
		if err := tx.First(&history, payload.HistoryID).Error; err != nil {
			return err
		}
		if history.Status != "waiting_approval" || history.CurrentStepID != payload.StepID {
			return errors.New("当前步骤不处于待确认状态或已被处理")
		}
		var err error
		definition, err = parseOpsJobDefinition(history.DefinitionJSON)
		if err != nil {
			return err
		}
		now := time.Now()
		status, decision, historyStatus, summary := "success", "approved", "running", "人工确认已通过"
		if !approved {
			status, decision, historyStatus, summary = "rejected", "rejected", "rejected", "作业被人工确认步骤拒绝"
		}
		updates := map[string]any{"status": historyStatus, "summary": summary}
		if !approved {
			updates["finished_at"] = &now
		}
		claim := tx.Model(&model.OpsJobHistory{}).Where("id = ? AND status = ? AND current_step_id = ?", history.ID, "waiting_approval", payload.StepID).Updates(updates)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errors.New("当前确认已被处理，请刷新页面")
		}
		var step model.OpsJobHistoryStep
		if err := tx.Where("history_id = ? AND step_id = ? AND status = ?", history.ID, payload.StepID, "waiting_approval").First(&step).Error; err != nil {
			return err
		}
		var duration int64
		if step.StartedAt != nil {
			duration = now.Sub(*step.StartedAt).Milliseconds()
		}
		return tx.Model(&step).Updates(map[string]any{"status": status, "summary": summary, "approval_decision": decision, "approval_note": Trimmed(payload.Note), "finished_at": &now, "duration_ms": duration}).Error
	})
	return definition, err
}
