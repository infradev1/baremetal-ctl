package main

import (
	"baremetal-ctl/proto"
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	// all RPC calls wait for conn.GetState() == connectivity.Ready

	client := proto.NewHelloServiceClient(conn)
	r, err := client.SayHello(ctx, &proto.SayHelloRequest{Name: "Charles"})
	// gRPC error handling best practice is to handle the non-nil errors from RPC calls
	if err != nil {
		log.Fatal(err)
	}
	log.Println(r.Message)
}
