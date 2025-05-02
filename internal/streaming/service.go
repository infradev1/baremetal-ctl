package streaming

import (
	"baremetal-ctl/proto"
	"fmt"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	proto.UnimplementedStreamingServiceServer
}

// StreamServerTime creates a copy of the server per request
func (s Service) StreamServerTime(req *proto.StreamServerTimeRequest, stream grpc.ServerStreamingServer[proto.StreamServerTimeResponse]) error {
	if req.GetIntervalSeconds() == 0 {
		return status.Error(codes.InvalidArgument, "seconds interval must be > 0")
	}

	interval := time.Duration(req.GetIntervalSeconds()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			res := &proto.StreamServerTimeResponse{
				CurrentTime: timestamppb.New(time.Now()),
			}
			if err := stream.Send(res); err != nil {
				return fmt.Errorf("error sending response message: %w", err)
			}
		}
	}
}

func (s Service) LogStream(stream proto.StreamingService_LogStreamServer) error {
	count := 0

	for {
		logReq, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed the stream
				return stream.SendAndClose(&proto.LogStreamResponse{
					EntriesLogged: int32(count),
				})
			}
			return err
		}

		log.Printf("received log [%s]: %s - %s", logReq.GetTimestamp().AsTime(), logReq.GetLevel().String(), logReq.GetMessage())
		count++
	}
}

func (s Service) Echo(stream proto.StreamingService_EchoServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // client closed the stream
				return nil // close server stream
			}
			return err
		}

		log.Printf("echoing message: %s", req.GetMessage())

		if err := stream.Send(&proto.EchoResponse{Message: req.GetMessage()}); err != nil {
			return err
		}
	}
}
