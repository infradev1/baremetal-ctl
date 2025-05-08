package file

import (
	"baremetal-ctl/cmd/server/file/server"
	"baremetal-ctl/internal"
	"baremetal-ctl/proto"
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestUploadFile_Success(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	addr := make(chan string)

	go server.NewFileServer(":0", NewService()).Start(ctx, addr)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	conn, err := grpc.NewClient(<-addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := proto.NewFileManagerClient(conn)
	fn := upload(ctx, client)
	assert.NotEmpty(t, fn)
}

func upload(ctx context.Context, client proto.FileManagerClient) string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(dir, "../../", "gopher.png")

	data, err := os.ReadFile(path)
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
