package plugin

import (
	"context"
	"fmt"

	pb "github.com/ShadowSmallBaby/ClawProxyHub/sdk/proto/cphv1"
)

type unavailableRuntime struct{}

func defaultRuntime(*Manager) Runtime            { return unavailableRuntime{} }
func (unavailableRuntime) Available(string) bool { return false }
func (unavailableRuntime) Start(context.Context, string, pb.ClawHostServer) (Session, error) {
	return nil, fmt.Errorf("Android requires a verified Service runtime")
}
