package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ops-admin/backend/model"
)

type SyncNode struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	FullValue string `json:"full_value"`
	ParentID  string `json:"parentId"`
}

// JumpServer represents related objects as either UUIDs or {id}/{pk} objects.
type syncReference string

func (r *syncReference) UnmarshalJSON(data []byte) error {
	var id string
	if json.Unmarshal(data, &id) == nil {
		*r = syncReference(id)
		return nil
	}
	var obj struct {
		ID string `json:"id"`
		PK string `json:"pk"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if obj.ID == "" {
		obj.ID = obj.PK
	}
	*r = syncReference(obj.ID)
	return nil
}

type SyncAsset struct {
	Cloud     *cloudInstance  `json:"cloud,omitempty"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Address   string          `json:"address"`
	Nodes     []syncReference `json:"nodes"`
	Protocols []struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	} `json:"protocols"`
	Active bool `json:"is_active"`
}

func normalizeSyncURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("请输入有效的 JumpServer HTTP/HTTPS 地址，不要包含账号、查询参数或片段")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func signJumpServerRequest(req *http.Request, credential model.AssetCredential) {
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	req.Header.Set("Accept", "application/json")
	canonical := "(request-target): get " + req.URL.RequestURI() + "\naccept: application/json\ndate: " + date + "\nhost: " + req.URL.Host
	mac := hmac.New(sha256.New, []byte(credential.Password))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("Authorization", fmt.Sprintf(`Signature keyId="%s",algorithm="hmac-sha256",headers="(request-target) accept date host",signature="%s"`, credential.Username, base64.StdEncoding.EncodeToString(mac.Sum(nil))))
}

// Never follow a server-provided pagination URL or redirect carrying API credentials.
// The caller's context bounds the entire snapshot, not just one page.
func jumpServerList[T any](ctx context.Context, source model.AssetSyncSource, credential model.AssetCredential, endpoint string) ([]T, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	items := make([]T, 0)
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		u := source.URL + endpoint + "?limit=200&offset=" + strconv.Itoa(len(items))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, errors.New("JumpServer 请求地址无效")
		}
		req.Header.Set("X-JMS-ORG", source.OrgID)
		signJumpServerRequest(req, credential)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("JumpServer 连接失败（%s），请检查地址、证书和网络：%w", endpoint, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("JumpServer 接口 %s 返回 HTTP %d，请检查组织、API 权限和地址", endpoint, resp.StatusCode)
		}
		if readErr != nil || len(data) > 16*1024*1024 {
			return nil, errors.New("JumpServer 响应读取失败或单页数据过大")
		}
		var batch []T
		if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
			if page != 0 {
				return nil, errors.New("JumpServer 分页响应格式发生变化")
			}
			if err := json.Unmarshal(data, &batch); err != nil {
				return nil, errors.New("JumpServer 列表格式无效")
			}
			return batch, nil
		}
		var envelope struct {
			Count   *int    `json:"count"`
			Next    *string `json:"next"`
			Results []T     `json:"results"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.Count == nil || envelope.Results == nil {
			return nil, errors.New("JumpServer 分页响应缺少 count/results，请核对 API 版本")
		}
		batch = envelope.Results
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		if seen[hash] {
			return nil, errors.New("JumpServer 返回重复分页，中止同步以避免不完整导入")
		}
		seen[hash] = true
		items = append(items, batch...)
		if len(items) == *envelope.Count && envelope.Next == nil {
			return items, nil
		}
		if len(batch) == 0 || len(items) >= *envelope.Count {
			return nil, errors.New("JumpServer 分页数量不一致，请重新预览")
		}
	}
	return nil, errors.New("JumpServer 资产超过本次同步上限（200000），请缩小接口账号权限范围")
}

func resolveSyncNodeParents(nodes []SyncNode) error {
	byKey := map[string]string{}
	ids := map[string]bool{}
	for _, n := range nodes {
		if n.ID == "" || n.Key == "" || ids[n.ID] || byKey[n.Key] != "" {
			return errors.New("JumpServer 节点 ID/key 缺失或重复")
		}
		ids[n.ID] = true
		byKey[n.Key] = n.ID
	}
	for i := range nodes {
		n := &nodes[i]
		if n.Name == "" {
			n.Name = n.Value
		}
		if n.Name == "" {
			return errors.New("JumpServer 节点名称为空")
		}
		if p := strings.LastIndex(n.Key, ":"); p >= 0 {
			n.ParentID = byKey[n.Key[:p]]
			if n.ParentID == "" {
				return errors.New("JumpServer 节点树不完整，请检查读取权限")
			}
		}
	}
	return nil
}
