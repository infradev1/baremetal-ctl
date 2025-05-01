package main

import (
	"baremetal-ctl/proto"
	"log"
)

func main() {
	p := proto.Person{
		Name: "Charles",
	}

	log.Println(p.GetName())

	//proto.HelloServiceServer.SayHello()
}
