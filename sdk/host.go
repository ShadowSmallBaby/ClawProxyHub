// host.go — 可取消的宿主回调，向调用方保留读取与持久化失败。
package sdk

import (
	"context"
	"errors"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

func (h *Host) callbackClient(ctx context.Context) (pb.ClawHostClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if h != nil {
		if client := h.conn(); client != nil {
			return client, nil
		}
	}
	return nil, errors.New("host callbacks unavailable")
}

// StoreGetContext 区分键不存在与读取失败，并跟随当前调用取消。
func (h *Host) StoreGetContext(ctx context.Context, key string) ([]byte, bool, error) {
	client, err := h.callbackClient(ctx)
	if err != nil {
		return nil, false, err
	}
	resp, err := client.StoreGet(ctx, &pb.StoreGetRequest{Key: key})
	if err != nil {
		return nil, false, err
	}
	if resp == nil {
		return nil, false, errors.New("empty StoreGet response")
	}
	if !resp.Found {
		return nil, false, nil
	}
	return resp.Value, true, nil
}

// StorePutContext 等待核心完成持久化，并返回写入失败。
func (h *Host) StorePutContext(ctx context.Context, key string, value []byte) error {
	client, err := h.callbackClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.StorePut(ctx, &pb.StorePutRequest{Key: key, Value: value})
	return err
}

// SettingsContext 读取插件级设置，保留宿主错误。
func (h *Host) SettingsContext(ctx context.Context, pluginName string) ([]byte, error) {
	return h.InstanceSettingsContext(ctx, pluginName, 0)
}

// InstanceSettingsContext 读取实例合并设置，instanceID 为 0 时只读插件设置。
func (h *Host) InstanceSettingsContext(ctx context.Context, pluginName string, instanceID int64) ([]byte, error) {
	client, err := h.callbackClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetSettings(ctx, &pb.GetSettingsRequest{Plugin: pluginName, InstanceId: instanceID})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty GetSettings response")
	}
	return resp.Values, nil
}
