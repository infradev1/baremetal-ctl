package main

import (
	"baremetal-ctl/internal"
	"baremetal-ctl/proto"
	"context"
	"io"
	"log"
	"os"
	"time"

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

	client := proto.NewFileManagerClient(conn)

	//g, ctx := errgroup.WithContext(ctx)

	fn := Upload(ctx, client)
	log.Printf("successfully uploaded %s to server", fn)

	n := Download(ctx, client, fn)
	log.Printf("successfully downloaded %d bytes from server", n)
}

func Upload(ctx context.Context, client proto.FileManagerClient) string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	data, err := os.ReadFile(dir + "/gopher.png")
	if err != nil {
		log.Fatal(err)
	}

	stream, err := client.UploadFile(ctx)
	if err != nil {
		log.Fatal(err)
	}

	chunkSize := 5 * 1024 // 5 KB
	chunks := internal.SplitIntoChunks(data, chunkSize)

	for _, chunk := range chunks {
		if err := stream.Send(&proto.UploadRequest{Chunk: chunk}); err != nil {
			log.Fatal(err)
		}
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatal(err)
	}

	return res.GetFileName()
}

func Download(ctx context.Context, client proto.FileManagerClient, fn string) int {
	stream, err := client.DownloadFile(ctx, &proto.DownloadRequest{FileName: fn})
	if err != nil {
		log.Fatal(err)
	}

	data := make([]byte, 0)

	for {
		res, err := stream.Recv()
		if err != nil {
			if err == io.EOF { // server closed stream
				break
			}
			log.Fatal(err)
		}
		data = append(data, res.GetChunk()...)
	}

	return len(data)
}
