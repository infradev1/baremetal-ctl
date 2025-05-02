package main

import (
	"baremetal-ctl/internal/streaming"
	"baremetal-ctl/proto"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const addr = ":50051"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("error running application", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("closing server gracefully")

	// grpcurl -d '{"name": "Charles"}' -import-path ./proto -proto hello.proto -plaintext localhost:50051 hello.v1.HelloService/SayHello
}

func run(ctx context.Context) error {
	server := grpc.NewServer()
	svc := streaming.Service{}
	proto.RegisterStreamingServiceServer(server, svc)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on address %q: %w", addr, err)
		}

		slog.Info("starting gRPC server", slog.String("address", addr))

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
