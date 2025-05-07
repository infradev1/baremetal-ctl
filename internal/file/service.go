package file

import (
	"baremetal-ctl/internal"
	"baremetal-ctl/proto"
	"fmt"
	"io"
	"log"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

func (s *Service) UploadFile(stream grpc.ClientStreamingServer[proto.UploadRequest, proto.UploadResponse]) error {
	fn := fmt.Sprintf("%s.png", uuid.New().String())
	bytes := 0

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed stream
				log.Printf("successfully uploaded %s to server: %d bytes", fn, bytes)

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
