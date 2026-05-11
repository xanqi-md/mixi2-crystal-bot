package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type DailyCardPayload struct {
	Text      string  `json:"text"`
	CardName  string  `json:"card_name"`
	ImageURL  string  `json:"image_url"`
	SourceURL *string `json:"source_url"`
	DateKey   string  `json:"date_key"`
}

func main() {
	log.Println("=== mixi2 Daily Card Poster ===")

	payloadPath := envOrDefault("DAILY_CARD_PAYLOAD_PATH", "/tmp/daily_card_payload.json")
	payload, err := loadPayload(payloadPath)
	if err != nil {
		log.Fatalf("payload読み込み失敗: %v", err)
	}

	communityID := os.Getenv("COMMUNITY_ID")
	if communityID == "" {
		log.Fatal("COMMUNITY_ID が設定されていません")
	}

	authenticator, err := auth.NewAuthenticator(
		os.Getenv("CLIENT_ID"),
		os.Getenv("CLIENT_SECRET"),
		os.Getenv("TOKEN_URL"),
	)
	if err != nil {
		log.Fatalf("認証初期化失敗: %v", err)
	}

	accessToken, err := authenticator.GetAccessToken(context.Background())
	if err != nil {
		log.Fatalf("アクセストークン取得失敗: %v", err)
	}

	authCtx, err := authenticator.AuthorizedContext(context.Background())
	if err != nil {
		log.Fatalf("認証コンテキスト取得失敗: %v", err)
	}

	apiConn, err := grpc.NewClient(
		os.Getenv("API_ADDRESS"),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatalf("API接続失敗: %v", err)
	}
	defer apiConn.Close()

	client := application_apiv1.NewApplicationServiceClient(apiConn)

	imageBytes, contentType, err := downloadImage(payload.ImageURL)
	if err != nil {
		log.Fatalf("画像ダウンロード失敗: %v", err)
	}
	log.Printf("画像取得成功: %s (%d bytes, %s)", payload.ImageURL, len(imageBytes), contentType)

	mediaID, err := uploadPostImage(authCtx, client, accessToken, payload.CardName, imageBytes, contentType)
	if err != nil {
		log.Fatalf("画像アップロード失敗: %v", err)
	}
	log.Printf("メディア準備完了: media_id=%s", mediaID)

	resp, err := client.CreatePost(authCtx, &application_apiv1.CreatePostRequest{
		Text:        payload.Text,
		CommunityId: &communityID,
		MediaIdList: []string{mediaID},
	})
	if err != nil {
		log.Fatalf("ポスト投稿失敗: %v", err)
	}

	log.Printf("投稿成功: post_id=%s community_id=%s text=%q", resp.GetPost().GetPostId(), communityID, payload.Text)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadPayload(path string) (*DailyCardPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload DailyCardPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Text == "" {
		return nil, fmt.Errorf("payload.text が空です")
	}
	if payload.CardName == "" {
		return nil, fmt.Errorf("payload.card_name が空です")
	}
	if payload.ImageURL == "" {
		return nil, fmt.Errorf("payload.image_url が空です")
	}
	return &payload, nil
}

func downloadImage(url string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "mixi2-crystal-bot/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return body, contentType, nil
}

func uploadPostImage(ctx context.Context, client application_apiv1.ApplicationServiceClient, accessToken, description string, data []byte, contentType string) (string, error) {
	initResp, err := client.InitiatePostMediaUpload(ctx, &application_apiv1.InitiatePostMediaUploadRequest{
		ContentType: contentType,
		DataSize:    uint64(len(data)),
		MediaType:   application_apiv1.InitiatePostMediaUploadRequest_TYPE_IMAGE,
		Description: &description,
	})
	if err != nil {
		return "", err
	}

	if err := uploadBinary(initResp.GetUploadUrl(), accessToken, contentType, data); err != nil {
		return "", err
	}

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		statusResp, err := client.GetPostMediaStatus(ctx, &application_apiv1.GetPostMediaStatusRequest{
			MediaId: initResp.GetMediaId(),
		})
		if err != nil {
			return "", err
		}

		switch statusResp.GetStatus() {
		case application_apiv1.GetPostMediaStatusResponse_STATUS_COMPLETED:
			return initResp.GetMediaId(), nil
		case application_apiv1.GetPostMediaStatusResponse_STATUS_FAILED:
			return "", fmt.Errorf("メディア処理が失敗しました")
		default:
			log.Printf("メディア処理待機中: media_id=%s status=%s", initResp.GetMediaId(), statusResp.GetStatus().String())
			time.Sleep(5 * time.Second)
		}
	}

	return "", fmt.Errorf("メディア処理がタイムアウトしました")
}

func uploadBinary(uploadURL, accessToken, contentType string, data []byte) error {
	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
