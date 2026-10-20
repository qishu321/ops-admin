package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const aliyunECSAPIEndpoint = "https://ecs.aliyuncs.com/"

type aliyunRPCError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type aliyunDescribeRegionsResponse struct {
	Regions struct {
		Region []struct {
			RegionID string `json:"RegionId"`
		} `json:"Region"`
	} `json:"Regions"`
}

type aliyunDescribeInstancesResponse struct {
	TotalCount int `json:"TotalCount"`
	Instances  struct {
		Instance []aliyunECSInstance `json:"Instance"`
	} `json:"Instances"`
}

type aliyunECSInstance struct {
	InstanceID      string `json:"InstanceId"`
	InstanceName    string `json:"InstanceName"`
	CPU             int    `json:"Cpu"`
	Memory          int    `json:"Memory"`
	OSName          string `json:"OSName"`
	RegionID        string `json:"RegionId"`
	PublicIPAddress struct {
		IPAddress []string `json:"IpAddress"`
	} `json:"PublicIpAddress"`
	InnerIPAddress struct {
		IPAddress []string `json:"IpAddress"`
	} `json:"InnerIpAddress"`
	EIPAddresses struct {
		IPAddress string `json:"IpAddress"`
	} `json:"EipAddress"`
	VPCAttributes struct {
		PrivateIPAddress struct {
			IPAddress []string `json:"IpAddress"`
		} `json:"PrivateIpAddress"`
	} `json:"VpcAttributes"`
	NetworkInterfaces struct {
		NetworkInterface []struct {
			PrimaryIPAddress string `json:"PrimaryIpAddress"`
			PrivateIPSets    struct {
				PrivateIPSet []struct {
					PrivateIPAddress string `json:"PrivateIpAddress"`
				} `json:"PrivateIpSet"`
			} `json:"PrivateIpSets"`
		} `json:"NetworkInterface"`
	} `json:"NetworkInterfaces"`
	SystemDisk struct {
		Size int `json:"Size"`
	} `json:"SystemDisk"`
	DataDisks struct {
		Disk []struct {
			Size int `json:"Size"`
		} `json:"Disk"`
	} `json:"DataDisks"`
}

func fetchAliyunCloudInstances(accessKey, secretKey, region string) ([]cloudInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return fetchAliyunCloudInstancesWithContext(ctx, accessKey, secretKey, region)
}
func fetchAliyunCloudInstancesWithContext(ctx context.Context, accessKey, secretKey, region string) ([]cloudInstance, error) {
	regions := normalizeCloudRegions(nil, region)
	if len(regions) == 0 {
		var response aliyunDescribeRegionsResponse
		if err := aliyunECSRequest(ctx, accessKey, secretKey, "DescribeRegions", "", nil, &response); err != nil {
			return nil, fmt.Errorf("unable to discover AliCloud regions: %w", err)
		}
		regions = regions[:0]
		for _, item := range response.Regions.Region {
			if strings.TrimSpace(item.RegionID) != "" {
				regions = append(regions, item.RegionID)
			}
		}
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("no AliCloud region available")
	}

	result := make([]cloudInstance, 0)
	for _, regionID := range regions {
		regionCount := 0
		for page := 1; page <= 100; page++ {
			params := url.Values{}
			params.Set("PageNumber", strconv.Itoa(page))
			params.Set("PageSize", "100")
			var response aliyunDescribeInstancesResponse
			if err := aliyunECSRequest(ctx, accessKey, secretKey, "DescribeInstances", regionID, params, &response); err != nil {
				return nil, fmt.Errorf("AliCloud ECS instance query failed for %s: %w", regionID, err)
			}
			result = append(result, aliyunCloudInstances(response.Instances.Instance, regionID)...)
			regionCount += len(response.Instances.Instance)
			if regionCount == response.TotalCount {
				break
			}
			if len(response.Instances.Instance) == 0 || regionCount > response.TotalCount || page == 100 {
				return nil, fmt.Errorf("AliCloud ECS pagination incomplete in %s; retry with a smaller scope", regionID)
			}
		}
	}
	return result, nil
}

func aliyunECSRequest(ctx context.Context, accessKey, secretKey, action, region string, extra url.Values, output any) error {
	params := url.Values{}
	params.Set("Format", "JSON")
	params.Set("Version", "2014-05-26")
	params.Set("AccessKeyId", accessKey)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", fmt.Sprintf("ops-admin-%d", time.Now().UnixNano()))
	params.Set("Action", action)
	if strings.TrimSpace(region) != "" {
		params.Set("RegionId", strings.TrimSpace(region))
	}
	for key, values := range extra {
		for _, value := range values {
			params.Add(key, value)
		}
	}
	params.Set("Signature", finOpsAliCloudSignature(params, secretKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aliyunECSEndpoint(region)+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	var apiError aliyunRPCError
	_ = json.Unmarshal(body, &apiError)
	if response.StatusCode < 200 || response.StatusCode >= 300 || apiError.Code != "" {
		return fmt.Errorf("%s: %s", firstNonEmpty(apiError.Code, response.Status), firstNonEmpty(apiError.Message, string(body)))
	}
	if err := json.Unmarshal(body, output); err != nil {
		return err
	}
	return nil
}

func aliyunECSEndpoint(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return aliyunECSAPIEndpoint
	}
	return "https://ecs." + region + ".aliyuncs.com/"
}

func aliyunCloudInstances(instances []aliyunECSInstance, fallbackRegion string) []cloudInstance {
	result := make([]cloudInstance, 0, len(instances))
	for _, item := range instances {
		privateIP := firstNonEmpty(
			firstAliyunIPAddress(item.VPCAttributes.PrivateIPAddress.IPAddress),
			firstAliyunIPAddress(item.InnerIPAddress.IPAddress),
		)
		if privateIP == "" && len(item.NetworkInterfaces.NetworkInterface) > 0 {
			primaryNIC := item.NetworkInterfaces.NetworkInterface[0]
			privateIP = strings.TrimSpace(primaryNIC.PrimaryIPAddress)
			if privateIP == "" {
				for _, privateIPSet := range primaryNIC.PrivateIPSets.PrivateIPSet {
					if privateIP = strings.TrimSpace(privateIPSet.PrivateIPAddress); privateIP != "" {
						break
					}
				}
			}
		}
		publicIP := firstNonEmpty(firstAliyunIPAddress(item.PublicIPAddress.IPAddress), item.EIPAddresses.IPAddress)
		diskGB := item.SystemDisk.Size
		for _, disk := range item.DataDisks.Disk {
			diskGB += disk.Size
		}
		result = append(result, cloudInstance{
			InstanceID: item.InstanceID,
			HostName:   firstNonEmpty(item.InstanceName, item.InstanceID),
			PrivateIP:  privateIP,
			PublicIP:   publicIP,
			CPU:        formatAliyunCPU(item.CPU),
			Memory:     formatAliyunMemory(item.Memory),
			Disk:       formatAliyunDisk(diskGB),
			OS:         item.OSName,
			Region:     firstNonEmpty(item.RegionID, fallbackRegion),
			SSHUser:    "root",
			SSHPort:    22,
		})
	}
	return result
}

func firstAliyunIPAddress(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatAliyunCPU(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d vCPU", value)
}

func formatAliyunMemory(valueMB int) string {
	if valueMB <= 0 {
		return ""
	}
	return fmt.Sprintf("%d GB", valueMB/1024)
}

func formatAliyunDisk(valueGB int) string {
	if valueGB <= 0 {
		return ""
	}
	return fmt.Sprintf("%d GB", valueGB)
}
