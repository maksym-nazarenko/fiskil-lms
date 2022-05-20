package pubsub

import (
	"context"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func StartServer(ctx context.Context, logger lms.Logger) *pstest.Server {
	// server part
	srv := pstest.NewServer()
	go func() {
		defer srv.Close()
		defer logger.Info("shutting down")
		<-ctx.Done()
	}()

	return srv
}

func NewClient(ctx context.Context, serverAddress, project string) (*pubsub.Client, func(), error) {
	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, func() {}, err
	}
	cancel := func() {
		defer conn.Close()
	}

	client, err := pubsub.NewClient(ctx, project, option.WithGRPCConn(conn))
	if err != nil {
		return nil, cancel, err
	}
	cancel = func() {
		defer conn.Close()
		defer client.Close()
	}

	return client, cancel, nil
}
