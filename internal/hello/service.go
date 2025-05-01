package hello

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// must implement type HelloServiceServer interface public methods
type Service struct {
	proto.UnimplementedHelloServiceServer // default
}

func (s *Service) SayHello(ctx context.Context, r *proto.SayHelloRequest) (*proto.SayHelloResponse, error) {
	if r.GetName() == "" {
		// errors.New("...") also possible for less error handling client side
		return nil, status.Error(codes.InvalidArgument, "name cannot be empty")
	}

	return &proto.SayHelloResponse{Message: fmt.Sprintf("Hello %s!", r.GetName())}, nil
}
