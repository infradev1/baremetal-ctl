package main

import (
	"baremetal-ctl/internal/hello"
	"baremetal-ctl/internal/todo"
	"baremetal-ctl/proto"
	"log"
	"net"

	"google.golang.org/grpc"
)

func main() {
	server := grpc.NewServer()
	// register gRPC service(s) on the server (RPCs automatically exposed)
	proto.RegisterHelloServiceServer(server, new(hello.Service))
	proto.RegisterTodoServiceServer(server, new(todo.Service))
	// 50051 is the standard port in gRPC
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("starting gRPC server on address %s", lis.Addr().String())

	// blocking function starts the gRPC server
	if err := server.Serve(lis); err != nil {
		log.Fatal(err)
	}
	// grpcurl -d '{"name": "Charles"}' -import-path ./proto -proto hello.proto -plaintext localhost:50051 hello.v1.HelloService/SayHello
}
