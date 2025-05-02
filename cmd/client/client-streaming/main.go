package main

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	// all RPC calls wait for conn.GetState() == connectivity.Ready

	client := proto.NewStreamingServiceClient(conn)

	stream, err := client.LogStream(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for i := range 10 {
		if err := stream.Send(&proto.LogStreamRequest{
			Timestamp: timestamppb.New(time.Now()),
			Level:     proto.LogLevel_LOG_LEVEL_INFO,
			Message:   fmt.Sprintf("log message #%d", i),
		}); err != nil {
			log.Fatal(err)
		}
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("total entries logged by server: %d", res.GetEntriesLogged())
}
