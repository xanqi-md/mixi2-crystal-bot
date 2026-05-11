package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	fmt.Println("=== mixi2 Plugin Post Bot ===")
	
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	fmt.Printf("現在時刻（UTC）: %s\n", now.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("明日の日付: %d日\n", tomorrow.Day())
	fmt.Printf("月末判定: %v\n", tomorrow.Day() == 1)
	
	// 認証の設定
	authenticator, err := auth.NewAuthenticator(
		os.Getenv("CLIENT_ID"),
		os.Getenv("CLIENT_SECRET"),
		os.Getenv("TOKEN_URL"),
	)
	if err != nil {
		log.Fatal("認証初期化失敗:", err)
	}
	fmt.Println("✓ 認証初期化成功")

	// API サーバーへの接続
	apiConn, err := grpc.NewClient(
		os.Getenv("API_ADDRESS"),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatal("API接続失敗:", err)
	}
	defer apiConn.Close()
	fmt.Println("✓ API接続成功")

	// API クライアントの作成
	client := application_apiv1.NewApplicationServiceClient(apiConn)
	fmt.Println("✓ クライアント作成成功")

	// 認証済みコンテキストの取得
	authCtx, err := authenticator.AuthorizedContext(context.Background())
	if err != nil {
		log.Fatal("認証コンテキスト取得失敗:", err)
	}
	fmt.Println("✓ 認証コンテキスト取得成功")

	// 月末判定
	if tomorrow.Day() == 1 {
		fmt.Println("\n=== 月末投稿を実行します ===")
		
		communityID := os.Getenv("COMMUNITY_ID")
		if communityID == "" {
			log.Fatal("COMMUNITY_ID が設定されていません")
		}
		fmt.Printf("投稿先コミュニティ ID: %s\n", communityID)
		
		// Plugin の場合、community_id を指定（文字列ポインタで渡す）
		resp, err := client.CreatePost(authCtx, &application_apiv1.CreatePostRequest{
			Text:        "【今日は月末です！ミッションの消化をお忘れなく！】",
			CommunityId: &communityID,
		})
		if err != nil {
			log.Fatal("ポスト投稿失敗:", err)
		}
		fmt.Printf("✓ ポスト投稿成功\n")
		fmt.Printf("  Post ID: %s\n", resp.Post.PostId)
		fmt.Printf("  投稿者: %s\n", resp.Post.CreatorId)
		fmt.Printf("  投稿時刻: %s\n", resp.Post.CreatedAt)
		fmt.Printf("  投稿内容: %s\n", resp.Post.Text)
		fmt.Printf("  投稿先コミュニティ: %s\n", resp.Post.CommunityId)
	} else {
		fmt.Printf("月末ではありません（明日は %d 日）\n", tomorrow.Day())
	}
}
