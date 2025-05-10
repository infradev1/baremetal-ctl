package server

import (
	"baremetal-ctl/proto"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	prom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type FileServer struct {
	address string
	limiter *rate.Limiter
	service proto.FileManagerServer
}

func NewFileServer(addr string, svc proto.FileManagerServer) *FileServer {
	return &FileServer{
		// Standard gRPC address is :50051 (:0 binds to a free port)
		address: addr,
		// Global rate limiter (100 requests/sec, burst of 10)
		limiter: rate.NewLimiter(rate.Limit(100), 10),
		// Dependencies
		service: svc,
	}
}

func (fs *FileServer) Start(ctx context.Context, ch chan<- string) {
	if err := fs.Run(ctx, ch); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("error running application", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("closing server gracefully")
}

func (fs *FileServer) Run(ctx context.Context, ch chan<- string) error {
	// TLS
	//tls, err := credentials.NewServerTLSFromFile("certs/server.crt", "certs/server.key")
	//if err != nil {
	//	// %w wraps the error such that it can later be unwrapped with errors.Unwrap, and so that it can be considered with errors.Is and errors.As
	//	return fmt.Errorf("failed to load TLS credentials: %w", err)
	//}

	serverCert, err := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
	if err != nil {
		return fmt.Errorf("failed to load server cert and key: %w", err)
	}

	caCert, err := os.ReadFile("certs/ca.crt")
	if err != nil {
		return fmt.Errorf("failed to load CA cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCert) {
		return errors.New("failed to append CA cert to pool")
	}

	tls := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS
	})

	// An interceptor is a function that wraps around the execution of an RPC.
	// It's the gRPC-native mechanism to add cross-cutting concerns like:
	// Authentication/authorization
	// Logging
	// Rate limiting
	// Metrics
	// Tracing
	// Panic recovery
	// In Go's gRPC world, interceptors are the middleware. Unlike HTTP frameworks (e.g. Chi, Echo)
	// where "middleware" is stacked via chaining, here we hook into a fixed spot per call type.
	server := grpc.NewServer(
		grpc.Creds(tls),
		grpc.UnaryInterceptor(prom.UnaryServerInterceptor),
		grpc.StreamInterceptor(fs.CompositeStreamInterceptor),
	)
	proto.RegisterFileManagerServer(server, fs.service)
	// only registers internal gRPC metrics — it doesn't expose them (hooks into the gRPC server to collect metrics)
	// Prometheus automatically tracks a variety of gRPC-level metrics, such as:
	// grpc_server_started_total: Number of RPCs started on the server
	// grpc_server_handled_total: Number of RPCs completed on the server
	// grpc_server_msg_received_total: Number of stream messages received
	// grpc_server_msg_sent_total: Number of stream messages sent
	// grpc_server_handling_seconds_bucket: Histogram of RPC handling durations
	prom.Register(server)
	// expose Prometheus metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:    ":9092",
		Handler: mux,
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		lis, err := net.Listen("tcp", fs.address)
		if err != nil {
			return fmt.Errorf("failed to listen on address %q: %w", fs.address, err)
		}

		slog.Info("starting gRPC server", slog.String("address", lis.Addr().String()))
		go func() {
			// send-only channel (blocks until a receiver consumes)
			ch <- lis.Addr().String()
		}()

		// blocking function (in a separate goroutine) starts the gRPC server
		if err := server.Serve(lis); err != nil {
			return fmt.Errorf("failed to serve gRPC service: %w", err)
		}

		return nil
	})

	g.Go(func() error {
		slog.Info("starting Prometheus metrics endpoint", slog.String("address", srv.Addr))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("metrics server error: %w", err)
		}
		return nil

	})

	g.Go(func() error {
		// wait on the Done channel of the Context (blocks in its own goroutine)
		<-ctx.Done()
		// continues executing when the Context is cancelled

		slog.Info("shutting down Prometheus metrics server")
		if err := srv.Shutdown(context.Background()); err != nil {
			slog.Warn("error shutting down metrics server", slog.String("error", err.Error()))
		}

		slog.Info("gracefully stopping gRPC server")
		server.GracefulStop()

		return nil
	})

	return g.Wait()
}

// CompositeStreamInterceptor checks rate limits for streaming RPCs and sets up Prometheus.
// Limits how many streams a client can open.
func (fs *FileServer) CompositeStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if !fs.limiter.Allow() {
		return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
	}
	return prom.StreamServerInterceptor(srv, ss, info, handler)
}
