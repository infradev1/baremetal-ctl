package file

import (
	"baremetal-ctl/internal"
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Service struct {
	proto.UnimplementedFileManagerServer
	sync.Mutex
	files map[string][]byte
}

func NewService() *Service {
	svc := new(Service)
	svc.files = make(map[string][]byte)
	return svc
}

// client-side streaming RPC
func (s *Service) UploadFile(stream grpc.ClientStreamingServer[proto.UploadRequest, proto.UploadResponse]) error {
	fn := fmt.Sprintf("%s.png", uuid.New().String())
	bytes := 0

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed stream
				slog.Info("successfully uploaded %s to server: %d bytes", fn, bytes)

				return stream.SendAndClose(&proto.UploadResponse{FileName: fn})
			}
			return status.Error(codes.Unknown, err.Error())
		}
		if len(req.GetChunk()) == 0 {
			return status.Error(codes.InvalidArgument, "file chunk size must be greater than 0")
		}

		s.Lock()
		s.files[fn] = append(s.files[fn], req.GetChunk()...)
		s.Unlock()

		slog.Info(fmt.Sprintf("uploaded %d bytes to server...", len(req.GetChunk())))
		//time.Sleep(1 * time.Second) // useful for debugging
		bytes += len(req.GetChunk())
	}
}

// server-side streaming RPC
func (s *Service) DownloadFile(req *proto.DownloadRequest, stream grpc.ServerStreamingServer[proto.DownloadResponse]) error {
	if req.GetFileName() == "" {
		return status.Error(codes.InvalidArgument, "file name must be provided")
	}

	file, ok := s.files[req.GetFileName()]
	if !ok {
		return status.Error(codes.NotFound, fmt.Sprintf("file name %s not found", req.GetFileName()))
	}

	const chunkSize = 5 * 1024 // 5 KB
	chunks := internal.SplitIntoChunks(file, chunkSize)

	for _, chunk := range chunks {
		if err := stream.Send(&proto.DownloadResponse{Chunk: chunk}); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

// bidirectional streaming RPC
func (s *Service) Echo(stream grpc.BidiStreamingServer[proto.EchoRequest, proto.EchoResponse]) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed the stream
				return nil // close server stream
			}
			return err
		}

		slog.Info("echoing", slog.String("message", req.GetMessage()))

		if err := stream.Send(&proto.EchoResponse{Message: req.GetMessage()}); err != nil {
			return err
		}
	}
}

// unary RPC (context use enforced)
func (s *Service) SayHello(ctx context.Context, r *proto.SayHelloRequest) (*proto.SayHelloResponse, error) {
	start := time.Now()

	if r.GetName() == "" {
		// errors.New("...") also possible for less error handling client side
		return nil, status.Error(codes.InvalidArgument, "name cannot be empty")
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok && len(md.Get("x-request-id")) > 0 {
		slog.Info("request header", slog.String("x-request-id", md.Get("x-request-id")[0]))
	}

	header := metadata.New(map[string]string{
		"request-start-timestamp": start.String(),
	})
	if err := grpc.SendHeader(ctx, header); err != nil {
		return nil, status.Error(codes.Internal, "failed to send header from server to client")
	}

	trailer := metadata.New(map[string]string{
		"request-end-timestamp": time.Now().String(),
	})
	if err := grpc.SetTrailer(ctx, trailer); err != nil {
		return nil, status.Error(codes.Internal, "failed to send trailer from server to client")
	}

	return &proto.SayHelloResponse{Message: fmt.Sprintf("Hello %s!", r.GetName())}, nil
}
