package main

import (
	"baremetal-ctl/cmd/server/file/server"
	"baremetal-ctl/internal/file"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	s := server.NewFileServer(":50051", file.NewService())

	if err := s.Run(ctx, make(chan string)); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("error running application", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
