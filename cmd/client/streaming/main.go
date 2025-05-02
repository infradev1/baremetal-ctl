package main

import (
	"baremetal-ctl/proto"
	"context"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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

	stream, err := client.StreamServerTime(ctx, &proto.StreamServerTimeRequest{
		IntervalSeconds: 5,
	})
	if err != nil {
		log.Fatal(err)
	}

	for {
		res, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			if status.Code(err) == codes.DeadlineExceeded {
				log.Println("context deadline exceeded")
				break
			}
			log.Fatal(err)
		}
		log.Printf("received time from server: %s", res.GetCurrentTime().AsTime())
	}

	log.Println("server stream closed gracefully")
}
