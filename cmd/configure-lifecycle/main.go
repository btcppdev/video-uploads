package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/zalando/go-keyring"
)

const (
	ruleID         = "btcpp-video-abort-incomplete-multipart"
	keyringService = "btcpp-video-uploader"
)

type settingsFile struct {
	Settings struct {
		Endpoint  string `json:"endpoint"`
		Region    string `json:"region"`
		Bucket    string `json:"bucket"`
		AccessKey string `json:"accessKey"`
	} `json:"settings"`
}

func main() {
	apply := flag.Bool("apply", false, "write the merged lifecycle configuration")
	days := flag.Int("days", 7, "days before incomplete multipart uploads are aborted")
	flag.Parse()
	if *days < 1 {
		fatalf("days must be at least 1")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		fatalf("find user config: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "btcpp-video", "queue.json"))
	if err != nil {
		fatalf("read uploader settings: %v", err)
	}
	var state settingsFile
	if err := json.Unmarshal(data, &state); err != nil {
		fatalf("parse uploader settings: %v", err)
	}
	s := state.Settings
	s.Bucket = strings.ToLower(strings.TrimSpace(s.Bucket))
	accessKey := strings.TrimSpace(os.Getenv("SPACES_ADMIN_KEY"))
	secret := os.Getenv("SPACES_ADMIN_SECRET")
	credentialSource := "one-time admin environment variables"
	if accessKey == "" || secret == "" {
		accessKey = s.AccessKey
		secret, err = keyring.Get(keyringService, "spaces-secret")
		credentialSource = "desktop uploader keychain credentials"
		if err != nil {
			fatalf("read Spaces secret from keychain: %v", err)
		}
	}
	if s.Endpoint == "" || s.Region == "" || s.Bucket == "" || accessKey == "" || secret == "" {
		fatalf("uploader Spaces settings are incomplete")
	}

	cfg := aws.Config{Region: s.Region, Credentials: credentials.NewStaticCredentialsProvider(accessKey, secret, "")}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s.Endpoint)
		o.UsePathStyle = false
	})
	ctx := context.Background()
	existing, err := client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(s.Bucket)})
	rules := []s3types.LifecycleRule{}
	if err != nil && !noLifecycleConfiguration(err) {
		fatalf("read lifecycle configuration for %q: %v", s.Bucket, err)
	}
	if existing != nil {
		rules = append(rules, existing.Rules...)
	}

	filtered := rules[:0]
	for _, rule := range rules {
		if aws.ToString(rule.ID) != ruleID {
			filtered = append(filtered, rule)
		}
	}
	rules = append(filtered, s3types.LifecycleRule{
		ID:     aws.String(ruleID),
		Status: s3types.ExpirationStatusEnabled,
		Filter: &s3types.LifecycleRuleFilter{Prefix: aws.String("")},
		AbortIncompleteMultipartUpload: &s3types.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: aws.Int32(int32(*days)),
		},
	})

	fmt.Printf("Bucket: %s\n", s.Bucket)
	fmt.Printf("Credentials: %s\n", credentialSource)
	fmt.Printf("Existing lifecycle rules preserved: %d\n", len(filtered))
	fmt.Printf("Rule: %s (abort incomplete multipart uploads after %d days)\n", ruleID, *days)
	if !*apply {
		fmt.Println("Dry run only; pass -apply to write this configuration.")
		return
	}
	_, err = client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 aws.String(s.Bucket),
		LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{Rules: rules},
	})
	if err != nil {
		fatalf("write lifecycle configuration for %q: %v", s.Bucket, err)
	}
	fmt.Println("Lifecycle configuration applied successfully.")
}

func noLifecycleConfiguration(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := strings.ToLower(apiErr.ErrorCode())
	return code == "nosuchlifecycleconfiguration" || code == "notfound" || code == "404"
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
