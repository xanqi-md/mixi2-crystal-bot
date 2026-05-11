package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"

	appv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: post <message> <token>")
	}

	message := os.Args[1]
	token := os.Args[2]
	apiServer := os.Getenv("MIXI_API_SERVER")
	if apiServer == "" {
		apiServer = "application-api.mixi.social:443"
	}

	fmt.Printf("メッセージ投稿: %s\n", message)

	conn, err := grpc.NewClient(apiServer, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		log.Fatal("gRPC 接続失敗:", err)
	}
	defer conn.Close()

	client := appv1.NewApplicationServiceClient(conn)

	// メタデータに認証トークンを追加
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)

	resp, err := client.CreatePost(ctx, &appv1.CreatePostRequest{Text: message})
	if err != nil {
		log.Fatal("投稿失敗:", err)
	}

	fmt.Printf("✓ 投稿成功 - Post ID: %s\n", resp.Post.PostId)
}
