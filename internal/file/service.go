package file

import (
	"baremetal-ctl/proto"
	"fmt"
	"io"
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

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed stream
				if err := stream.SendAndClose(&proto.UploadResponse{
					FileName: fn,
				}); err != nil {
					return status.Error(codes.Internal, err.Error())
				}
				return nil
			}
			return status.Error(codes.Unknown, err.Error())
		}
		if len(req.GetChunk()) == 0 {
			return status.Error(codes.InvalidArgument, "file chunk size must be greater than 0")
		}

		s.Lock()
		s.files[fn] = append(s.files[fn], req.GetChunk()...)
		s.Unlock()
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

	chunkSize := 5 * 1024 // 5 KB
	chunks := splitIntoChunks(file, chunkSize)

	for _, chunk := range chunks {
		if err := stream.Send(&proto.DownloadResponse{Chunk: chunk}); err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

func splitIntoChunks(data []byte, chunkSize int) [][]byte {
	var chunks [][]byte

	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		chunks = append(chunks, data[i:end])
	}

	return chunks
}
