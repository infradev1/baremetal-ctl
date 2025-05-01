package hello

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
)

// must implement type HelloServiceServer interface public methods
type Service struct {
	proto.UnimplementedHelloServiceServer // default
}

func (s *Service) SayHello(ctx context.Context, r *proto.SayHelloRequest) (*proto.SayHelloResponse, error) {
	return &proto.SayHelloResponse{Message: fmt.Sprintf("Hello %s!", r.GetName())}, nil
}
