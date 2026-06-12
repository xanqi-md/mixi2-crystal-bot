package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mixigroup/mixi2-application-sdk-go/auth"
	application_apiv1 "github.com/mixigroup/mixi2-application-sdk-go/gen/go/social/mixi/application/service/application_api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func maskSecret(s string) string {
	if len(s) <= 6 {
		return "***"
	}
	return s[:6] + strings.Repeat("*", len(s)-6)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)
	log.Println("=== OpenSkill Rating Bot - Connection Test ===")

	clientID := envOrDefault("CLIENT_ID", "")
	if clientID == "" {
		clientID = envOrDefault("MIXI_CLIENT_ID", "")
	}
	clientSecret := envOrDefault("CLIENT_SECRET", "")
	if clientSecret == "" {
		clientSecret = envOrDefault("MIXI_CLIENT_SECRET", "")
	}
	tokenURL := envOrDefault("TOKEN_URL", "https://application-auth.mixi.social/oauth2/token")
	apiAddress := envOrDefault("API_ADDRESS", "application-api.mixi.social:443")
	adminUserID := envOrDefault("ADMIN_USER_ID", "")
	if adminUserID == "" {
		adminUserID = envOrDefault("MIXI_ADMIN_USER_ID", "openskill_rating")
	}

	log.Printf("CLIENT_ID prefix : %s", maskSecret(clientID))
	log.Printf("TOKEN_URL        : %s", tokenURL)
	log.Printf("API_ADDRESS      : %s", apiAddress)
	log.Printf("ADMIN_USER_ID    : %s", adminUserID)

	if clientID == "" || clientSecret == "" {
		log.Fatal("❌ CLIENT_ID / CLIENT_SECRET not set")
	}

	// Step 1: Token
	log.Println("[1/3] Getting access token...")
	authenticator, err := auth.NewAuthenticator(clientID, clientSecret, tokenURL)
	if err != nil {
		log.Fatalf("❌ Authenticator failed: %v", err)
	}
	tokenCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	token, err := authenticator.GetAccessToken(tokenCtx)
	if err != nil {
		log.Fatalf("❌ Token failed: %v", err)
	}
	log.Printf("✅ Token: %s...", maskSecret(token))

	// Step 2: gRPC connection
	log.Println("[2/3] Connecting to API...")
	conn, err := grpc.NewClient(apiAddress, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	if err != nil {
		log.Fatalf("❌ gRPC failed: %v", err)
	}
	defer conn.Close()
	log.Printf("✅ gRPC connected to %s", apiAddress)

	// Step 3: API call
	log.Println("[3/3] Testing API call...")
	client := application_apiv1.NewApplicationServiceClient(conn)
	authCtx, err := authenticator.AuthorizedContext(context.Background())
	if err != nil {
		log.Fatalf("❌ AuthorizedContext failed: %v", err)
	}
	apiCtx, apiCancel := context.WithTimeout(authCtx, 15*time.Second)
	defer apiCancel()

	resp, err := client.GetUsers(apiCtx, &application_apiv1.GetUsersRequest{
		UserIdList: []string{adminUserID},
	})
	if err != nil {
		log.Fatalf("❌ GetUsers failed: %v", err)
	}
	if len(resp.Users) > 0 {
		u := resp.Users[0]
		log.Printf("✅ Admin user: id=%s display_name=%s", u.GetUserId(), u.GetDisplayName())
	} else {
		log.Printf("✅ API call succeeded (user %s not found, but connection OK)", adminUserID)
	}

	fmt.Println("\n========================================")
	fmt.Println("✅ ALL CHECKS PASSED - Ready to deploy!")
	fmt.Println("========================================")
}
