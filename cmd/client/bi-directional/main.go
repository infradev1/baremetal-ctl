package main

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := proto.NewStreamingServiceClient(conn)

	g, ctx := errgroup.WithContext(ctx)

	stream, err := client.Echo(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// client goroutine
	g.Go(func() error {
		for i := range 10 {
			if err := stream.Send(&proto.EchoRequest{
				Message: fmt.Sprintf("message #%d", i),
			}); err != nil {
				return err
			}
		}

		if err := stream.CloseSend(); err != nil {
			return err
		}
		return nil
	})

	// server goroutine
	g.Go(func() error {
		for {
			res, err := stream.Recv()
			if err != nil {
				if err == io.EOF { // server stream closed
					break
				}
				return err
			}
			log.Println(res.GetMessage())
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Fatal(err)
	}

	log.Println("bidirectional stream closed")
}
