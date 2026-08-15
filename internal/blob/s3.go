package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const maxBlobRead = 150 * 1024 * 1024

type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Prefix          string
	UsePathStyle    bool
}

func NewS3(ctx context.Context, input S3Config) (*S3Store, error) {
	if strings.TrimSpace(input.Bucket) == "" {
		return nil, fmt.Errorf("必须配置 S3 存储桶")
	}
	region := input.Region
	if region == "" {
		region = "us-east-1"
	}
	options := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if input.AccessKeyID != "" || input.SecretAccessKey != "" {
		if input.AccessKeyID == "" || input.SecretAccessKey == "" {
			return nil, fmt.Errorf("必须同时配置 S3 访问密钥和秘密密钥")
		}
		options = append(options, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(input.AccessKeyID, input.SecretAccessKey, input.SessionToken)))
	}
	awsConfig, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("加载 S3 配置失败：%w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = input.UsePathStyle || input.Endpoint != ""
		if input.Endpoint != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(input.Endpoint, "/"))
		}
	})
	return &S3Store{client: client, bucket: input.Bucket, prefix: strings.Trim(strings.TrimSpace(input.Prefix), "/")}, nil
}

func NewFromEnv(ctx context.Context, localRoot string) (Store, error) {
	bucket := strings.TrimSpace(os.Getenv("CONTENTCLOUD_S3_BUCKET"))
	if bucket == "" {
		return NewLocal(localRoot)
	}
	return NewS3(ctx, S3Config{
		Bucket:          bucket,
		Region:          os.Getenv("CONTENTCLOUD_S3_REGION"),
		Endpoint:        os.Getenv("CONTENTCLOUD_S3_ENDPOINT"),
		AccessKeyID:     os.Getenv("CONTENTCLOUD_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("CONTENTCLOUD_S3_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("CONTENTCLOUD_S3_SESSION_TOKEN"),
		Prefix:          os.Getenv("CONTENTCLOUD_S3_PREFIX"),
		UsePathStyle:    os.Getenv("CONTENTCLOUD_S3_PATH_STYLE") == "1",
	})
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte) error {
	objectKey, err := s.key(key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(s.bucket),
		Key:                  aws.String(objectKey),
		Body:                 bytes.NewReader(data),
		ContentLength:        aws.Int64(int64(len(data))),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("写入 S3 对象失败：%w", err)
	}
	return nil
}

func (s *S3Store) PutReader(ctx context.Context, key string, reader io.Reader, size int64) error {
	objectKey, err := s.key(key)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey), Body: reader, ServerSideEncryption: types.ServerSideEncryptionAes256}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("写入 S3 对象失败：%w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	objectKey, err := s.key(key)
	if err != nil {
		return nil, err
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)})
	if err != nil {
		var notFound *types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("读取 S3 对象失败：%w", err)
	}
	defer result.Body.Close()
	if result.ContentLength != nil && *result.ContentLength > maxBlobRead {
		return nil, fmt.Errorf("S3 对象超过读取大小限制")
	}
	data, err := io.ReadAll(io.LimitReader(result.Body, maxBlobRead+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBlobRead {
		return nil, fmt.Errorf("S3 对象超过读取大小限制")
	}
	return data, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	objectKey, err := s.key(key)
	if err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(objectKey)}); err != nil {
		return fmt.Errorf("删除 S3 对象失败：%w", err)
	}
	return nil
}

func (s *S3Store) key(key string) (string, error) {
	clean := strings.Trim(strings.ReplaceAll(key, "\\", "/"), "/")
	if clean == "" || strings.Contains(clean, "../") || strings.HasPrefix(clean, "..") || strings.ContainsRune(clean, '\x00') {
		return "", fmt.Errorf("对象键无效")
	}
	if s.prefix == "" {
		return clean, nil
	}
	return s.prefix + "/" + clean, nil
}
