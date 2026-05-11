package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
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
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)
	log.Println("=== mixi2 Daily Card Poster: start ===")

	payloadPath := envOrDefault("DAILY_CARD_PAYLOAD_PATH", "/tmp/daily_card_payload.json")
	log.Printf("[step 1/7] payload 読み込み開始: path=%s", payloadPath)

	payload, err := loadPayload(payloadPath)
	if err != nil {
		log.Fatalf("[fatal] payload読み込み失敗: %v", err)
	}
	log.Printf("[step 1/7] payload 読み込み成功: date_key=%s card_name=%q image_url=%s source_url=%s text_preview=%q",
		payload.DateKey,
		payload.CardName,
		payload.ImageURL,
		nilToString(payload.SourceURL),
		truncateString(payload.Text, 120),
	)

	communityID := os.Getenv("COMMUNITY_ID")
	clientID := os.Getenv("CLIENT_ID")
	clientSecret := os.Getenv("CLIENT_SECRET")
	tokenURL := os.Getenv("TOKEN_URL")
	apiAddress := os.Getenv("API_ADDRESS")

	mustEnv("COMMUNITY_ID", communityID)
	mustEnv("CLIENT_ID", clientID)
	mustEnv("CLIENT_SECRET", clientSecret)
	mustEnv("TOKEN_URL", tokenURL)
	mustEnv("API_ADDRESS", apiAddress)

	log.Printf("[step 2/7] 環境変数確認: community_id=%s api_address=%s token_url=%s client_id_prefix=%s",
		communityID,
		apiAddress,
		tokenURL,
		maskPrefix(clientID, 6),
	)

	log.Printf("[step 3/7] 認証初期化開始")
	authenticator, err := auth.NewAuthenticator(clientID, clientSecret, tokenURL)
	if err != nil {
		log.Fatalf("[fatal] 認証初期化失敗: %v", err)
	}
	log.Printf("[step 3/7] 認証初期化成功")

	tokenCtx, cancelToken := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelToken()

	accessToken, err := authenticator.GetAccessToken(tokenCtx)
	if err != nil {
		log.Fatalf("[fatal] アクセストークン取得失敗: %v", err)
	}
	log.Printf("[step 3/7] アクセストークン取得成功: token_prefix=%s", maskPrefix(accessToken, 10))

	authCtx, err := authenticator.AuthorizedContext(context.Background())
	if err != nil {
		log.Fatalf("[fatal] 認証コンテキスト取得失敗: %v", err)
	}
	log.Printf("[step 3/7] 認証コンテキスト取得成功")

	log.Printf("[step 4/7] gRPC 接続開始: %s", apiAddress)
	apiConn, err := grpc.NewClient(
		apiAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
	)
	if err != nil {
		log.Fatalf("[fatal] API接続失敗: %v", err)
	}
	defer apiConn.Close()
	log.Printf("[step 4/7] gRPC 接続成功")

	client := application_apiv1.NewApplicationServiceClient(apiConn)

	log.Printf("[step 5/7] 画像ダウンロード開始: %s", payload.ImageURL)
	imageBytes, contentType, err := downloadImage(payload.ImageURL)
	if err != nil {
		log.Fatalf("[fatal] 画像ダウンロード失敗: %v", err)
	}
	log.Printf("[step 5/7] 画像ダウンロード成功: bytes=%d content_type=%s", len(imageBytes), contentType)

	log.Printf("[step 6/7] メディアアップロード開始: card_name=%q", payload.CardName)
	mediaID, err := uploadPostImage(authCtx, client, accessToken, payload.CardName, imageBytes, contentType)
	if err != nil {
		log.Fatalf("[fatal] 画像アップロード失敗: %v", err)
	}
	log.Printf("[step 6/7] メディア準備完了: media_id=%s", mediaID)

	log.Printf("[step 7/7] 投稿作成開始: community_id=%s text_preview=%q media_id=%s",
		communityID,
		truncateString(payload.Text, 120),
		mediaID,
	)

	postCtx, cancelPost := context.WithTimeout(authCtx, 30*time.Second)
	defer cancelPost()

	resp, err := client.CreatePost(postCtx, &application_apiv1.CreatePostRequest{
		Text:        payload.Text,
		CommunityId: &communityID,
		MediaIdList: []string{mediaID},
	})
	if err != nil {
		log.Fatalf("[fatal] ポスト投稿失敗: %v", err)
	}

	if resp.GetPost() == nil {
		log.Fatalf("[fatal] 投稿レスポンス異常: post が nil")
	}

	log.Printf("[success] 投稿成功: post_id=%s community_id=%s created_at=%v text=%q",
		resp.GetPost().GetPostId(),
		communityID,
		resp.GetPost().GetCreatedAt(),
		payload.Text,
	)
	log.Println("=== mixi2 Daily Card Poster: done ===")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key, value string) {
	if strings.TrimSpace(value) == "" {
		log.Fatalf("[fatal] 必須環境変数が未設定です: %s", key)
	}
}

func loadPayload(path string) (*DailyCardPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("payload ファイル読込失敗: %w", err)
	}

	log.Printf("[debug] payload raw size=%d preview=%q", len(data), truncateString(string(data), 240))

	var payload DailyCardPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("payload JSON パース失敗: %w", err)
	}

	if strings.TrimSpace(payload.Text) == "" {
		return nil, fmt.Errorf("payload.text が空です")
	}
	if strings.TrimSpace(payload.CardName) == "" {
		return nil, fmt.Errorf("payload.card_name が空です")
	}
	if strings.TrimSpace(payload.ImageURL) == "" {
		return nil, fmt.Errorf("payload.image_url が空です")
	}

	return &payload, nil
}

func downloadImage(rawURL string) ([]byte, string, error) {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("リクエスト生成失敗: %w", err)
	}
	req.Header.Set("User-Agent", "mixi2-crystal-bot/1.0")

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("HTTP GET 失敗: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[debug] image GET response: status=%d content_length=%d duration=%s",
		resp.StatusCode,
		resp.ContentLength,
		time.Since(start),
	)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("レスポンス読込失敗: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("画像取得失敗: status=%d body_preview=%q", resp.StatusCode, truncateString(string(body), 240))
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	contentType = normalizeContentType(contentType)

	log.Printf("[debug] image bytes preview: first16=% x", firstN(body, 16))

	return body, contentType, nil
}

func uploadPostImage(
	ctx context.Context,
	client application_apiv1.ApplicationServiceClient,
	accessToken string,
	description string,
	data []byte,
	contentType string,
) (string, error) {
	initCtx, cancelInit := context.WithTimeout(ctx, 30*time.Second)
	defer cancelInit()

	log.Printf("[debug] InitiatePostMediaUpload: content_type=%s size=%d description=%q",
		contentType,
		len(data),
		description,
	)

	initResp, err := client.InitiatePostMediaUpload(initCtx, &application_apiv1.InitiatePostMediaUploadRequest{
		ContentType: contentType,
		DataSize:    uint64(len(data)),
		MediaType:   application_apiv1.InitiatePostMediaUploadRequest_TYPE_IMAGE,
		Description: &description,
	})
	if err != nil {
		return "", fmt.Errorf("InitiatePostMediaUpload 失敗: %w", err)
	}

	mediaID := initResp.GetMediaId()
	uploadURL := initResp.GetUploadUrl()

	if strings.TrimSpace(mediaID) == "" {
		return "", fmt.Errorf("InitiatePostMediaUpload の media_id が空です")
	}
	if strings.TrimSpace(uploadURL) == "" {
		return "", fmt.Errorf("InitiatePostMediaUpload の upload_url が空です")
	}

	log.Printf("[debug] InitiatePostMediaUpload 成功: media_id=%s upload_url_prefix=%s",
		mediaID,
		maskPrefix(uploadURL, 80),
	)

	if err := uploadBinary(uploadURL, accessToken, contentType, data); err != nil {
		return "", fmt.Errorf("バイナリアップロード失敗: %w", err)
	}

	deadline := time.Now().Add(2 * time.Minute)
	pollCount := 0

	for time.Now().Before(deadline) {
		pollCount++

		statusCtx, cancelStatus := context.WithTimeout(ctx, 20*time.Second)
		statusResp, err := client.GetPostMediaStatus(statusCtx, &application_apiv1.GetPostMediaStatusRequest{
			MediaId: mediaID,
		})
		cancelStatus()

		if err != nil {
			return "", fmt.Errorf("GetPostMediaStatus 失敗: %w", err)
		}

		status := statusResp.GetStatus().String()
		log.Printf("[debug] GetPostMediaStatus: poll=%d media_id=%s status=%s", pollCount, mediaID, status)

		switch statusResp.GetStatus() {
		case application_apiv1.GetPostMediaStatusResponse_STATUS_COMPLETED:
			return mediaID, nil
		case application_apiv1.GetPostMediaStatusResponse_STATUS_FAILED:
			return "", fmt.Errorf("メディア処理が失敗しました: media_id=%s", mediaID)
		default:
			time.Sleep(5 * time.Second)
		}
	}

	return "", fmt.Errorf("メディア処理がタイムアウトしました: media_id=%s timeout=%s", mediaID, 2*time.Minute)
}

func uploadBinary(uploadURL, accessToken, contentType string, data []byte) error {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("アップロード用リクエスト生成失敗: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(data))

	log.Printf("[debug] upload binary request: url_prefix=%s bytes=%d content_type=%s",
		maskPrefix(uploadURL, 80),
		len(data),
		contentType,
	)

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("アップロードHTTP失敗: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[debug] upload binary response: status=%d duration=%s body_preview=%q",
		resp.StatusCode,
		time.Since(start),
		truncateString(string(body), 240),
	)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: status=%d body=%s", resp.StatusCode, truncateString(string(body), 500))
	}

	return nil
}

func normalizeContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return contentType
	}
	return mediaType
}

func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func maskPrefix(s string, visible int) string {
	if s == "" {
		return ""
	}
	if visible <= 0 {
		return "***"
	}
	if len(s) <= visible {
		return s
	}
	return s[:visible] + "***"
}

func nilToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func firstN(b []byte, n int) []byte {
	if n <= 0 {
		return []byte{}
	}
	if len(b) <= n {
		return b
	}
	return b[:n]
}
