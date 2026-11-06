package service

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ops-admin/backend/model"
)

// Serializes admission within this server; persisted active runs also block later requests.
var pipelineAdmission sync.Mutex

func (s *Service) ensurePipelineIdle(appID uint, env string) error {
	var count int64
	if err := s.db.Model(&model.OpsAppPipelineRun{}).Where("app_id = ? AND env = ? AND status IN ?", appID, env, []string{"running", "waiting_approval"}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该应用环境已有执行中或待审批的发布，请等待结束")
	}
	return nil
}

func sshPipelineHostIDs(config map[string]any) []uint {
	b, _ := json.Marshal(config["hostIds"])
	var ids []uint
	_ = json.Unmarshal(b, &ids)
	return uniqueIDs(ids)
}

func validateSSHPipelineConfig(config map[string]any) error {
	root := opsPipelineConfigString(config, "deployRoot")
	if !path.IsAbs(root) || path.Clean(root) == "/" || path.Clean(root) != root || strings.ContainsAny(root, "\r\n\\") {
		return errors.New("部署目录必须是非根目录的规范 Linux 绝对路径")
	}
	if len(sshPipelineHostIDs(config)) == 0 {
		return errors.New("请选择部署目标主机")
	}
	if opsPipelineConfigString(config, "artifactPath") == "" {
		return errors.New("请填写执行节点上的 tar.gz 制品路径")
	}
	if opsPipelineConfigString(config, "activateScript") == "" || opsPipelineConfigString(config, "healthScript") == "" {
		return errors.New("请填写启动命令和健康检查命令；它们也会用于回滚验证")
	}
	return nil
}

func (s *Service) sshPipelineHosts(config map[string]any) ([]model.AssetHost, error) {
	ids := sshPipelineHostIDs(config)
	hosts, err := s.resolveOpsTargetHosts(ids, nil)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(hosts) {
		return nil, errors.New("部分目标主机不存在或不可访问")
	}
	for _, host := range hosts {
		if host.Status != 1 || host.CredentialID == nil || *host.CredentialID == 0 {
			return nil, errors.New("目标主机已停用或缺少 SSH 凭据")
		}
	}
	return hosts, nil
}

// Only regular files and directories are extracted. Links and parent paths can escape a release.
func validatePipelineArchive(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", errors.New("制品必须为 tar.gz 文件")
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	files := 0
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := path.Clean(h.Name)
		if name == ".artifact.tar.gz" || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(h.Name, "\\\r\n") {
			return "", errors.New("制品包含非法路径")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return "", errors.New("制品仅支持普通文件和目录，不支持链接及设备文件")
		}
		if h.Typeflag == tar.TypeReg {
			files++
		}
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return "", err
	}
	if files == 0 {
		return "", errors.New("制品中没有文件")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Service) pipelineSSHArtifact(execInfo opsPipelineExecution, stage OpsAppPipelineStageDefinition) (model.OpsAppArtifact, error) {
	var artifact model.OpsAppArtifact
	if execInfo.ArtifactID > 0 {
		err := s.db.Where("id = ? AND app_id = ? AND type = ? AND status = ?", execInfo.ArtifactID, execInfo.App.ID, "ssh-archive", "ready").First(&artifact).Error
		if err != nil {
			return artifact, errors.New("所选制品不是可用的 SSH 发布包")
		}
		digest, err := validatePipelineArchive(artifact.URI)
		if err != nil {
			return artifact, err
		}
		if digest != artifact.Digest {
			return artifact, errors.New("制品校验失败，文件内容已改变")
		}
		return artifact, nil
	}
	source := opsPipelineConfigString(stage.Config, "artifactPath")
	if !path.IsAbs(source) {
		source = path.Join(execInfo.Workspace, source)
	}
	_, staged, err := s.stageRemoteFile(execInfo.ExecutorHost.ID, source)
	if err != nil {
		return artifact, err
	}
	defer os.Remove(staged)
	digest, err := validatePipelineArchive(staged)
	if err != nil {
		return artifact, err
	}
	directory := filepath.Join("data", "pipeline-artifacts", fmt.Sprint(execInfo.RunID))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return artifact, err
	}
	key := sha256.Sum256([]byte(stage.ID))
	destination := filepath.Join(directory, fmt.Sprintf("%x.tar.gz", key[:8]))
	if err := os.Rename(staged, destination); err != nil {
		return artifact, err
	}
	var run model.OpsAppPipelineRun
	if err := s.db.First(&run, execInfo.RunID).Error; err != nil {
		return artifact, err
	}
	artifact = model.OpsAppArtifact{AppID: execInfo.App.ID, AppName: execInfo.App.Name, Env: execInfo.Env, Version: execInfo.ImageTag, CommitID: run.CommitID, Type: "ssh-archive", URI: destination, Digest: digest, Status: "ready"}
	if err := s.db.Create(&artifact).Error; err != nil {
		return artifact, err
	}
	if err := s.db.Model(&model.OpsAppPipelineRun{}).Where("id = ?", execInfo.RunID).Update("artifact_id", artifact.ID).Error; err != nil {
		return artifact, err
	}
	return artifact, nil
}

func (s *Service) pipelineHostCommand(host model.AssetHost, command string, seconds int) (string, error) {
	result := s.execCommandOnHost(host, command, seconds)
	output := strings.TrimSpace(result.Stdout + "\n" + result.Stderr)
	if result.Status != "success" {
		return output, errors.New(firstNonEmpty(result.ErrorText, output, "SSH 执行失败"))
	}
	return output, nil
}

func pipelineActivateCommand(root, release string, config map[string]any) string {
	// Rename a temporary symlink atomically; never replace a real current directory.
	return "set -e\nexport RELEASE_DIR=" + shellQuote(release) + "\nexport DEPLOY_ROOT=" + shellQuote(root) + "\ncd \"$DEPLOY_ROOT\"\nif [ -e current ] && [ ! -L current ]; then echo 'current must be a symlink'; exit 1; fi\nswitch_dir=$(mktemp -d \"$DEPLOY_ROOT/.switch-XXXXXX\")\ntrap 'rmdir \"$switch_dir\" 2>/dev/null || true' EXIT\nln -s \"$RELEASE_DIR\" \"$switch_dir/current\"\nmv -Tf \"$switch_dir/current\" current\ncd current\n" +
		"sh -ec " + shellQuote(opsPipelineConfigString(config, "activateScript")) + "\nsh -ec " + shellQuote(opsPipelineConfigString(config, "healthScript"))
}

func (s *Service) executeSSHPipelineDeploy(execInfo opsPipelineExecution, stage OpsAppPipelineStageDefinition) (string, error) {
	if err := validateSSHPipelineConfig(stage.Config); err != nil {
		return "", err
	}
	hosts, err := s.sshPipelineHosts(stage.Config)
	if err != nil {
		return "", err
	}
	artifact, err := s.pipelineSSHArtifact(execInfo, stage)
	if err != nil {
		return "", err
	}
	root := opsPipelineConfigString(stage.Config, "deployRoot")
	key := sha256.Sum256([]byte(stage.ID))
	release := path.Join(root, "releases", fmt.Sprintf("%d-%x", execInfo.RunID, key[:6]))
	seconds := normalizeOpsBuildTimeout(stage.TimeoutSeconds)
	var logs strings.Builder
	for _, host := range hosts {
		target := model.OpsAppPipelineTarget{RunID: execInfo.RunID, StageID: stage.ID, HostID: host.ID, HostName: host.HostName, Root: root, ReleasePath: release, Status: "preparing"}
		if err := s.db.Create(&target).Error; err != nil {
			return logs.String(), err
		}
		// Snapshot before any activation; refused if current is a directory or a broken symlink.
		previous, hostErr := s.pipelineHostCommand(host, "set -e\nmkdir -p "+shellQuote(root)+"\ncd "+shellQuote(root)+"\nif [ -L current ]; then readlink -f current; elif [ -e current ]; then echo 'current must be a symlink' >&2; exit 1; fi", seconds)
		target.PreviousPath = previous
		if hostErr == nil {
			hostErr = s.db.Model(&target).Update("previous_path", previous).Error
		}
		if hostErr == nil {
			var canonical string
			canonical, hostErr = s.pipelineHostCommand(host, "set -e\ntest ! -e "+shellQuote(release)+"\nmkdir -p "+shellQuote(release)+"\nreadlink -f "+shellQuote(release), seconds)
			if hostErr == nil {
				target.ReleasePath = canonical
				hostErr = s.db.Model(&target).Update("release_path", canonical).Error
			}
		}
		archive := path.Join(release, ".artifact.tar.gz")
		if hostErr == nil {
			result := s.dispatchFileFromPath(host, ".artifact.tar.gz", archive, artifact.URI, false, seconds)
			if result.Status != "success" {
				hostErr = errors.New(firstNonEmpty(result.ErrorText, "制品分发失败"))
			}
		}
		if hostErr == nil {
			_, hostErr = s.pipelineHostCommand(host, "set -e\necho "+shellQuote(artifact.Digest+"  "+archive)+" | sha256sum -c -\ntar --no-same-owner --no-same-permissions -xzf "+shellQuote(archive)+" -C "+shellQuote(release)+"\nrm -f "+shellQuote(archive), seconds)
		}
		if hostErr == nil {
			// Persist intent first: a timeout can occur after the symlink has switched.
			hostErr = s.db.Model(&target).Update("status", "activating").Error
			if hostErr == nil {
				var output strings.Builder
				var outputMu sync.Mutex
				lastPersist := time.Now()
				result := s.execCommandOnHostStreaming(host, pipelineActivateCommand(root, release, stage.Config), seconds, func(chunk string) {
					outputMu.Lock()
					defer outputMu.Unlock()
					output.WriteString(chunk)
					if time.Since(lastPersist) > 500*time.Millisecond {
						_ = s.db.Model(&target).Update("log", output.String()).Error
						lastPersist = time.Now()
					}
				})
				outputMu.Lock()
				target.Log = output.String()
				outputMu.Unlock()
				if result.Status != "success" {
					hostErr = errors.New(firstNonEmpty(result.ErrorText, "启动或健康检查失败"))
				}
			}
		}
		target.Status = "success"
		if hostErr != nil {
			target.Status = "failed"
			target.Log += "\n" + hostErr.Error()
		}
		if err := s.db.Model(&target).Updates(map[string]any{"status": target.Status, "log": target.Log}).Error; err != nil {
			return logs.String(), err
		}
		fmt.Fprintf(&logs, "%s [%s]\n%s\n", host.HostName, target.Status, target.Log)
		if hostErr != nil {
			return logs.String(), fmt.Errorf("主机 %s 发布失败：%w；后续主机未执行，可从执行详情回滚已切换主机", host.HostName, hostErr)
		}
	}
	return logs.String(), nil
}

func (s *Service) rollbackSSHPipeline(current model.OpsAppPipelineRun, operator string) (map[string]any, error) {
	if current.TriggerType == "rollback" {
		return nil, errors.New("回滚记录不能再次回滚，请选择原始发布记录")
	}
	var targets []model.OpsAppPipelineTarget
	if err := s.db.Where("run_id = ?", current.ID).Order("id DESC").Find(&targets).Error; err != nil {
		return nil, err
	}
	stages, _, err := normalizeOpsPipelineStages(current.DefinitionJSON)
	if err != nil {
		return nil, err
	}
	configs := map[string]OpsAppPipelineStageDefinition{}
	for _, stage := range stages {
		configs[stage.ID] = stage
	}
	for _, target := range targets {
		if _, err := s.getOpsPipelineExecutorHost(target.HostID); err != nil {
			return nil, err
		}
		if err := s.authorizeOpsTargets([]uint{target.HostID}, nil); err != nil {
			return nil, err
		}
	}
	now := time.Now()
	run := model.OpsAppPipelineRun{PipelineID: current.PipelineID, PipelineName: current.PipelineName, AppID: current.AppID, AppName: current.AppName, AppCode: current.AppCode, Env: current.Env, ImageTag: current.ImageTag, TriggerType: "rollback", TriggerUser: operator, Status: "running", Summary: fmt.Sprintf("恢复发布 #%d 各主机原版本", current.ID), StartedAt: &now, DefinitionJSON: current.DefinitionJSON}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, err
	}
	go func() {
		status, summary := "success", "已恢复各主机发布前的版本"
		for _, original := range targets {
			target := original
			target.ID = 0
			target.RunID = run.ID
			target.Status = "running"
			target.Log = ""
			if err := s.db.Create(&target).Error; err != nil {
				status, summary = "failed", err.Error()
				break
			}
			host, hostErr := s.getOpsPipelineExecutorHost(target.HostID)
			output := ""
			if hostErr == nil {
				var actual string
				actual, hostErr = s.pipelineHostCommand(host, "if [ -L "+shellQuote(path.Join(target.Root, "current"))+" ]; then readlink -f "+shellQuote(path.Join(target.Root, "current"))+"; fi", 30)
				if hostErr == nil {
					restore, decisionErr := pipelineRollbackDecision(actual, target.PreviousPath, target.ReleasePath)
					if decisionErr != nil {
						hostErr = decisionErr
					} else if !restore {
						output = "该主机未切换或已恢复，无需回滚"
					} else {
						stage := configs[target.StageID]
						output, hostErr = s.pipelineHostCommand(host, "test -d "+shellQuote(target.PreviousPath)+" && "+"sh -ec "+shellQuote(pipelineActivateCommand(target.Root, target.PreviousPath, stage.Config)), normalizeOpsBuildTimeout(stage.TimeoutSeconds))
					}
				}
			}
			target.Status = "success"
			if hostErr != nil {
				target.Status = "failed"
				status, summary = "failed", "部分主机恢复失败，请查看主机结果"
				output += "\n" + hostErr.Error()
			}
			if err := s.db.Model(&target).Updates(map[string]any{"status": target.Status, "log": output}).Error; err != nil {
				status, summary = "failed", err.Error()
			}
		}
		finished := time.Now()
		_ = s.db.Model(&run).Updates(map[string]any{"status": status, "summary": summary, "finished_at": &finished, "duration_ms": finished.Sub(now).Milliseconds()}).Error
	}()
	return map[string]any{"runId": run.ID}, nil
}

func pipelineRollbackDecision(actual, previous, released string) (bool, error) {
	if actual == previous {
		return false, nil
	}
	if actual != released {
		return false, errors.New("当前版本已改变，拒绝覆盖后续发布")
	}
	if previous == "" {
		return false, errors.New("首次发布没有可恢复的旧版本，请人工处理")
	}
	return true, nil
}
