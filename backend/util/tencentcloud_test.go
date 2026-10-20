package util

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

func TestTencentInventoryPagesAcrossRegions(t *testing.T) {
	var calls []string
	items, err := tencentInstancePages(context.Background(), []string{"ap-shanghai", "ap-beijing", "ap-shanghai"}, func(_ context.Context, region string, offset int) ([]TencentInstanceInfo, int, error) {
		calls = append(calls, fmt.Sprintf("%s:%d", region, offset))
		total := 205
		if region == "ap-beijing" {
			total = 1
		}
		var batch []TencentInstanceInfo
		for i := offset; i < offset+100 && i < total; i++ {
			batch = append(batch, TencentInstanceInfo{InstanceID: fmt.Sprintf("i-%d", i)})
		}
		return batch, total, nil
	})
	if err != nil || len(items) != 206 || !reflect.DeepEqual(calls, []string{"ap-shanghai:0", "ap-shanghai:100", "ap-shanghai:200", "ap-beijing:0"}) {
		t.Fatalf("items=%d calls=%v err=%v", len(items), calls, err)
	}
	if items[205].Region != "ap-beijing" {
		t.Fatal("region was lost")
	}
}

func TestTencentIncompleteInventoryNeverReturnsPartialSuccess(t *testing.T) {
	for _, mode := range []string{"region-failure", "early-empty", "duplicate", "count-changed", "missing-id"} {
		t.Run(mode, func(t *testing.T) {
			items, err := tencentInstancePages(context.Background(), []string{"one", "two"}, func(_ context.Context, region string, offset int) ([]TencentInstanceInfo, int, error) {
				if mode == "region-failure" {
					if region == "two" {
						return nil, 0, errors.New("network")
					}
					return []TencentInstanceInfo{{InstanceID: "i"}}, 1, nil
				}
				if mode == "missing-id" {
					return []TencentInstanceInfo{{}}, 1, nil
				}
				if offset == 0 {
					return []TencentInstanceInfo{{InstanceID: "i"}}, 2, nil
				}
				switch mode {
				case "early-empty":
					return nil, 2, nil
				case "duplicate":
					return []TencentInstanceInfo{{InstanceID: "i"}}, 2, nil
				default:
					return []TencentInstanceInfo{{InstanceID: "j"}}, 3, nil
				}
			})
			if err == nil || items != nil {
				t.Fatalf("partial snapshot escaped: %v %v", items, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tencentInstancePages(ctx, []string{"one"}, func(context.Context, string, int) ([]TencentInstanceInfo, int, error) {
		t.Fatal("canceled task fetched network")
		return nil, 0, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestTencentCapacityUnitsAndOptionalFields(t *testing.T) {
	item := tencentInstanceInfo(&cvm.Instance{Memory: common.Int64Ptr(8), CPU: common.Int64Ptr(4), SystemDisk: &cvm.SystemDisk{DiskSize: common.Int64Ptr(50)}, DataDisks: []*cvm.DataDisk{nil, {DiskSize: common.Int64Ptr(100)}}}, "ap-shanghai")
	if item.Memory != 8 || item.CPU != 4 || item.Disk != 150 {
		t.Fatalf("capacity was lost or memory converted twice: %#v", item)
	}
}
