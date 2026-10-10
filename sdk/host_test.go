package sdk

import (
	"context"
	"errors"
	"testing"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
	"google.golang.org/grpc"
)

type hostCallbackClient struct {
	pb.ClawHostClient
	get      func(context.Context, *pb.StoreGetRequest) (*pb.StoreGetResponse, error)
	put      func(context.Context, *pb.StorePutRequest) (*pb.Empty, error)
	settings func(context.Context, *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error)
}

func (c *hostCallbackClient) StoreGet(ctx context.Context, req *pb.StoreGetRequest, _ ...grpc.CallOption) (*pb.StoreGetResponse, error) {
	return c.get(ctx, req)
}

func (c *hostCallbackClient) StorePut(ctx context.Context, req *pb.StorePutRequest, _ ...grpc.CallOption) (*pb.Empty, error) {
	return c.put(ctx, req)
}

func (c *hostCallbackClient) GetSettings(ctx context.Context, req *pb.GetSettingsRequest, _ ...grpc.CallOption) (*pb.GetSettingsResponse, error) {
	return c.settings(ctx, req)
}

func TestHostCallbacksPreserveContextAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("callback failed")
	calls := 0
	checkContext := func(got context.Context) {
		t.Helper()
		calls++
		if got != ctx {
			t.Fatal("callback lost caller context")
		}
	}
	host := NewHost(&hostCallbackClient{
		get: func(got context.Context, req *pb.StoreGetRequest) (*pb.StoreGetResponse, error) {
			checkContext(got)
			if req.Key != "cursor" {
				t.Fatal(req)
			}
			return nil, failure
		},
		put: func(got context.Context, req *pb.StorePutRequest) (*pb.Empty, error) {
			checkContext(got)
			if req.Key != "cursor" || string(req.Value) != "next" {
				t.Fatal(req)
			}
			return nil, failure
		},
		settings: func(got context.Context, req *pb.GetSettingsRequest) (*pb.GetSettingsResponse, error) {
			checkContext(got)
			if req.Plugin != "example" || (req.InstanceId != 0 && req.InstanceId != 42) {
				t.Fatal(req)
			}
			return nil, failure
		},
	})
	operations := map[string]func(*Host) error{
		"get":      func(h *Host) error { _, _, err := h.StoreGetContext(ctx, "cursor"); return err },
		"put":      func(h *Host) error { return h.StorePutContext(ctx, "cursor", []byte("next")) },
		"settings": func(h *Host) error { _, err := h.SettingsContext(ctx, "example"); return err },
		"instance": func(h *Host) error { _, err := h.InstanceSettingsContext(ctx, "example", 42); return err },
	}
	for name, call := range operations {
		if err := call(host); !errors.Is(err, failure) {
			t.Fatalf("%s hid callback failure: %v", name, err)
		}
		if err := call(NewHost(nil)); err == nil {
			t.Fatalf("%s accepted unavailable callbacks", name)
		}
	}
	cancel()
	for name, call := range operations {
		if err := call(host); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s lost cancellation: %v", name, err)
		}
	}
	if calls != len(operations) {
		t.Fatalf("canceled calls reached the host: %d", calls)
	}
}

func TestHostStoreDistinguishesEmptyAndMissing(t *testing.T) {
	host := NewHost(&hostCallbackClient{
		get: func(_ context.Context, req *pb.StoreGetRequest) (*pb.StoreGetResponse, error) {
			return &pb.StoreGetResponse{Found: req.Key == "empty"}, nil
		},
	})
	for _, key := range []string{"empty", "missing"} {
		value, found, err := host.StoreGetContext(context.Background(), key)
		if err != nil || found != (key == "empty") || len(value) != 0 {
			t.Fatalf("%s: value=%q found=%v error=%v", key, value, found, err)
		}
	}
}
