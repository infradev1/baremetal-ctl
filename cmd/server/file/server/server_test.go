package server

import (
	"baremetal-ctl/internal"
	"baremetal-ctl/internal/file"
	"baremetal-ctl/proto"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGlobalRateLimit(t *testing.T) {
	go func() {
		ctx, cancel := signal.NotifyContext(context.Background(),
			os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
		)
		defer cancel()

		s := NewFileServer(file.NewService())

		if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("error running application", slog.String("error", err.Error()))
			os.Exit(1)
		}

		slog.Info("closing server gracefully")
	}()

	g, ctx := errgroup.WithContext(context.Background())

	for i := range 10 { // global burst of 10 server-side
		g.Go(func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second*1)
			defer cancel()

			conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return err
			}
			defer conn.Close()

			client := proto.NewFileManagerClient(conn)

			dir, err := os.Getwd()
			if err != nil {
				return err
			}

			path := filepath.Join(dir, "../../../../", "gopher.png")

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			fn, err := Upload(ctx, client, data)
			if err != nil {
				return err
			}

			slog.Info(fmt.Sprintf("successfully uploaded %s to server", fn), slog.Int("goroutine", i))
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		assert.Fail(t, "client goroutine failure below burst size", err)
	}

	slog.Info("first test passed")

	for i := range 20 { // 100 RPS with 10 burst size (N - 10 will be spaced out in time by the server)
		g.Go(func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second*1) // context will timeout for post-burst requests
			defer cancel()

			conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				return err
			}
			defer conn.Close()

			client := proto.NewFileManagerClient(conn)

			dir, err := os.Getwd()
			if err != nil {
				return err
			}

			path := filepath.Join(dir, "../../../../", "gopher.png")

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			fn, err := Upload(ctx, client, data)
			if err != nil {
				return err
			}

			slog.Info(fn, slog.Int("goroutine", i))
			return nil
		})
	}

	if err := g.Wait(); err == nil {
		assert.Fail(t, "client goroutines should time out", err)
	}
}

func Upload(ctx context.Context, client proto.FileManagerClient, file []byte) (string, error) {
	stream, err := client.UploadFile(ctx)
	if err != nil {
		return "", err
	}

	chunkSize := 5 * 1024 // 5 KB
	chunks := internal.SplitIntoChunks(file, chunkSize)

	for _, chunk := range chunks {
		if err := stream.Send(&proto.UploadRequest{Chunk: chunk}); err != nil {
			return "", err
		}
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		return "", err
	}

	return res.GetFileName(), nil
}
