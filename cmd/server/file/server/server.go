package server

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"log/slog"
	"net"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FileServer struct {
	address string
	limiter *rate.Limiter
	service proto.FileManagerServer
}

func NewFileServer(svc proto.FileManagerServer) *FileServer {
	return &FileServer{
		// Standard gRPC address
		address: ":50051",
		// Global rate limiter (100 requests/sec, burst of 10)
		limiter: rate.NewLimiter(rate.Limit(100), 10),
		// Dependencies
		service: svc,
	}
}

func (fs *FileServer) Run(ctx context.Context) error {
	server := grpc.NewServer(
		grpc.StreamInterceptor(fs.RateLimitStreamInterceptor),
	)
	proto.RegisterFileManagerServer(server, fs.service)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		lis, err := net.Listen("tcp", fs.address)
		if err != nil {
			return fmt.Errorf("failed to listen on address %q: %w", fs.address, err)
		}

		slog.Info("starting gRPC server", slog.String("address", fs.address))

		// blocking function (in a separate goroutine) starts the gRPC server
		if err := server.Serve(lis); err != nil {
			return fmt.Errorf("failed to serve gRPC service: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		// wait on the Done channel of the Context (blocks in its own goroutine)
		<-ctx.Done()
		// continues executing when the Context is cancelled
		server.GracefulStop()

		return nil
	})

	return g.Wait()
}

// RateLimitStreamInterceptor checks rate limits for streaming RPCs.
// Limits how many streams a client can open.
func (fs *FileServer) RateLimitStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if !fs.limiter.Allow() {
		return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
	}
	return handler(srv, ss)
}
