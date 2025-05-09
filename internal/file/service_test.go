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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var address = make(chan string)

const chunkSize = 5 * 1024 // 5 KB

func TestUploadFile_Success(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	go server.NewFileServer(":0", NewService()).Start(ctx, address)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	conn, err := grpc.NewClient(<-address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	defer conn.Close()

	client := proto.NewFileManagerClient(conn)

	stream, err := client.UploadFile(ctx)
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	chunks := internal.SplitIntoChunks(loadData(), chunkSize)

	for _, chunk := range chunks {
		if err := stream.Send(&proto.UploadRequest{Chunk: chunk}); err != nil {
			assert.FailNow(t, err.Error())
		}
	}

	res, err := stream.CloseAndRecv()
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	assert.NotEmpty(t, res.GetFileName())
}

func TestUploadFile_Failure(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	go server.NewFileServer(":0", NewService()).Start(ctx, address)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	conn, err := grpc.NewClient(<-address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		assert.FailNow(t, err.Error())
	}
	defer conn.Close()

	client := proto.NewFileManagerClient(conn)

	stream, err := client.UploadFile(ctx)
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	empty := make([]byte, 0)
	err = stream.Send(&proto.UploadRequest{Chunk: empty})
	if err != nil {
		assert.FailNow(t, err.Error())
	}

	_, err = stream.CloseAndRecv()
	if err != nil {
		assert.ErrorIs(t, err, status.Error(codes.InvalidArgument, "file chunk size must be greater than 0"))
	} else {
		assert.Fail(t, "expected error response from server due to empty chunk")
	}
}

func loadData() []byte {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(dir, "../../", "gopher.png")

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	return data
}
