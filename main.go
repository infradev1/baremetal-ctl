package main

import (
	"baremetal-ctl/internal/hello"
	"baremetal-ctl/proto"
	"context"
	"log"
)

func main() {
	svc := new(hello.Service)

	r, err := svc.SayHello(context.TODO(), &proto.SayHelloRequest{Name: "Charles"})
	if err != nil {
		log.Fatal(err)
	}

	log.Println(r.Message)
}
