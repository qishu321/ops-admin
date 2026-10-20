package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

type TencentCloudService struct{ AccessKey, AccessSecret string }
type TencentInstanceInfo struct {
	InstanceID, HostName, PrivateIP, PublicIP string
	CPU, Memory, Disk                         int // CVM Memory is GiB, not MiB.
	OS, Region                                string
}

func NewTencentCloudService(accessKey, accessSecret string) *TencentCloudService {
	return &TencentCloudService{AccessKey: accessKey, AccessSecret: accessSecret}
}
func (s *TencentCloudService) GetInstances(regions []string) ([]TencentInstanceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.GetInstancesWithContext(ctx, regions)
}
func (s *TencentCloudService) GetInstancesWithContext(ctx context.Context, regions []string) ([]TencentInstanceInfo, error) {
	credential := common.NewCredential(s.AccessKey, s.AccessSecret)
	clientProfile := profile.NewClientProfile()
	clientProfile.HttpProfile.Endpoint = "cvm.tencentcloudapi.com"
	clientProfile.HttpProfile.ReqTimeout = 15
	clients := map[string]*cvm.Client{}
	return tencentInstancePages(ctx, regions, func(ctx context.Context, region string, offset int) ([]TencentInstanceInfo, int, error) {
		client := clients[region]
		if client == nil {
			var err error
			client, err = cvm.NewClient(credential, region, clientProfile)
			if err != nil {
				return nil, 0, err
			}
			clients[region] = client
		}
		request := cvm.NewDescribeInstancesRequest()
		request.Limit = common.Int64Ptr(100)
		request.Offset = common.Int64Ptr(int64(offset))
		response, err := client.DescribeInstancesWithContext(ctx, request)
		if err != nil {
			return nil, 0, err
		}
		if response == nil || response.Response == nil || response.Response.TotalCount == nil {
			return nil, 0, fmt.Errorf("CVM response is missing TotalCount")
		}
		items := make([]TencentInstanceInfo, 0, len(response.Response.InstanceSet))
		for _, instance := range response.Response.InstanceSet {
			if instance != nil {
				items = append(items, tencentInstanceInfo(instance, region))
			}
		}
		return items, int(*response.Response.TotalCount), nil
	})
}

func tencentInstancePages(ctx context.Context, regions []string, fetch func(context.Context, string, int) ([]TencentInstanceInfo, int, error)) ([]TencentInstanceInfo, error) {
	if len(regions) == 0 {
		return nil, fmt.Errorf("at least one Tencent Cloud region is required")
	}
	all := []TencentInstanceInfo{}
	visitedRegions := map[string]bool{}
	for _, region := range regions {
		region = strings.TrimSpace(region)
		if region == "" || visitedRegions[region] {
			continue
		}
		visitedRegions[region] = true
		seen := map[string]bool{}
		offset := 0
		expected := -1
		for page := 0; page < 1000; page++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			batch, total, err := fetch(ctx, region, offset)
			if err != nil {
				return nil, fmt.Errorf("Tencent Cloud region %s: %w", region, err)
			}
			if total < 0 || expected >= 0 && expected != total {
				return nil, fmt.Errorf("CVM inventory changed during pagination in %s; retry preview", region)
			}
			expected = total
			for _, item := range batch {
				if item.InstanceID == "" || seen[item.InstanceID] {
					return nil, fmt.Errorf("CVM repeated or missing instance ID in %s", region)
				}
				seen[item.InstanceID] = true
				item.Region = region
				all = append(all, item)
			}
			offset += len(batch)
			if offset == total {
				break
			}
			if len(batch) == 0 || offset > total || page == 999 {
				return nil, fmt.Errorf("CVM pagination incomplete in %s", region)
			}
		}
	}
	if len(visitedRegions) == 0 {
		return nil, fmt.Errorf("at least one Tencent Cloud region is required")
	}
	return all, nil
}

func tencentInstanceInfo(instance *cvm.Instance, region string) TencentInstanceInfo {
	item := TencentInstanceInfo{Region: region}
	if instance.InstanceId != nil {
		item.InstanceID = *instance.InstanceId
	}
	if instance.InstanceName != nil {
		item.HostName = *instance.InstanceName
	}
	if instance.OsName != nil {
		item.OS = *instance.OsName
	}
	if instance.CPU != nil {
		item.CPU = int(*instance.CPU)
	}
	if instance.Memory != nil {
		item.Memory = int(*instance.Memory)
	}
	if instance.SystemDisk != nil && instance.SystemDisk.DiskSize != nil {
		item.Disk += int(*instance.SystemDisk.DiskSize)
	}
	for _, disk := range instance.DataDisks {
		if disk != nil && disk.DiskSize != nil {
			item.Disk += int(*disk.DiskSize)
		}
	}
	if len(instance.PrivateIpAddresses) > 0 && instance.PrivateIpAddresses[0] != nil {
		item.PrivateIP = *instance.PrivateIpAddresses[0]
	}
	if len(instance.PublicIpAddresses) > 0 && instance.PublicIpAddresses[0] != nil {
		item.PublicIP = *instance.PublicIpAddresses[0]
	}
	return item
}
