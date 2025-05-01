package main

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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

	//client := proto.NewHelloServiceClient(conn)
	//r, err := client.SayHello(ctx, &proto.SayHelloRequest{Name: "Charles"})
	client := proto.NewTodoServiceClient(conn)

	for i := range 20 {
		_, err := client.AddTask(ctx, &proto.AddTaskRequest{Task: fmt.Sprintf("task number %d", i+1)})
		// gRPC error handling best practice is to handle the non-nil errors from RPC calls client-side
		if err != nil {
			s, ok := status.FromError(err)
			if ok {
				log.Fatalf("status code: %s, error: %s", s.Code().String(), s.Message())
			}
			log.Fatal(err)
		}
	}

	res, _ := client.ListTasks(ctx, &proto.ListTasksRequest{})

	log.Printf("%d TODO tasks\n", len(res.GetTasks()))

	for _, task := range res.GetTasks() {
		log.Printf("Task %s: %s", task.GetId(), task.GetTask())

		if _, err := client.CompleteTask(ctx, &proto.CompleteTaskRequest{Id: task.GetId()}); err != nil {
			s, ok := status.FromError(err)
			if ok {
				log.Fatalf("status code: %s, error: %s", s.Code().String(), s.Message())
			}
			log.Fatal(err)
		}

	}

	res, _ = client.ListTasks(ctx, &proto.ListTasksRequest{})
	log.Printf("%d TODO tasks\n", len(res.GetTasks()))
}
