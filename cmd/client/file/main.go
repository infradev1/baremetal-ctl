package main

import (
	"baremetal-ctl/internal"
	"baremetal-ctl/proto"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	// if using a public CA
	//tls := credentials.NewTLS(&tls.Config{})
	// private
	//tls := credentials.NewClientTLSFromCert(certPool, "")

	certPool := x509.NewCertPool()
	caCert, err := os.ReadFile("certs/ca.crt")
	if err != nil {
		log.Fatal(err)
	}
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		log.Fatal("failed to append CA cert")
	}

	clientCert, err := tls.LoadX509KeyPair("certs/client.crt", "certs/client.key")
	if err != nil {
		log.Fatal(err)
	}

	tls := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
	})

	// In Kubernetes, the Service name would be used, which CoreDNS would resolve to an actual IP address
	conn, err := grpc.NewClient("localhost:50051",
		grpc.WithTransportCredentials(tls),
		grpc.WithChainUnaryInterceptor( // only applies to unary rpc calls
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				start := time.Now()
				err := invoker(ctx, method, req, reply, cc, opts...)                       // call next interceptor in the chain (logger)
				slog.Info("request time", slog.String(method, time.Since(start).String())) // includes logging time
				return err
			},
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				slog.Info("sending request", slog.String(method, fmt.Sprintf("%v", req)))
				err := invoker(ctx, method, req, reply, cc, opts...)
				slog.Info("received response", slog.String(method, fmt.Sprintf("%v", reply)))
				return err
			},
		),
		// optional: add client-side streaming chained interceptors
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := proto.NewFileManagerClient(conn)

	fn := Upload(ctx, client)
	log.Printf("successfully uploaded %s to server", fn)

	n := Download(ctx, client, fn)
	log.Printf("successfully downloaded %d bytes from server", n)

	res, err := client.SayHello(ctx, &proto.SayHelloRequest{Name: "Charles"})
	if err != nil {
		log.Fatal(err)
	}
	slog.Info(res.GetMessage())

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

	slog.Info("bidirectional stream closed")
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
