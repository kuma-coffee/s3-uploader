package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	Bucket = "direct-uploads"
	Region = "us-east-1"

	AccessKey = "minioadmin"
	SecretKey = "minioadmin123"

	Endpoint = "http://localhost:9000"

	ServerPort = ":8080"
)

func initMinioBucket(ctx context.Context, client *s3.Client) error {

	log.Println("Checking MinIO connection...")

	_, err := client.ListBuckets(
		ctx,
		&s3.ListBucketsInput{},
	)
	if err != nil {
		return fmt.Errorf("cannot connect to MinIO: %w", err)
	}

	log.Println("MinIO connection OK")

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(Bucket),
	})
	if err == nil {
		log.Printf("Bucket '%s' already exists\n", Bucket)
		return nil
	}

	log.Printf("Bucket '%s' not found, creating...\n", Bucket)

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(Bucket),
	})
	if err != nil {
		return fmt.Errorf("failed create bucket: %w", err)
	}

	log.Printf("Bucket '%s' created successfully\n", Bucket)

	return nil
}

func main() {

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				AccessKey,
				SecretKey,
				"",
			),
		),
	)

	if err != nil {
		log.Fatal("AWS config error:", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(Endpoint)
		o.UsePathStyle = true
	})

	// cek MinIO + bucket
	if err := initMinioBucket(ctx, client); err != nil {
		log.Fatal(err)
	}

	presignClient := s3.NewPresignClient(client)

	r := gin.Default()

	r.Use(cors.New(cors.Config{

		AllowOrigins: []string{
			"http://localhost:5173",
		},

		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
			"OPTIONS",
		},

		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
		},

		AllowCredentials: true,

		MaxAge: 12 * time.Hour,
	}))

	// Generate presigned upload URL
	r.POST("/upload-url", func(c *gin.Context) {

		fileName := c.Query("filename")

		if fileName == "" {
			c.JSON(400, gin.H{
				"error": "filename wajib diisi",
			})
			return
		}

		objectKey := fmt.Sprintf(
			"%s-%s",
			uuid.New().String(),
			fileName,
		)

		result, err := presignClient.PresignPutObject(
			ctx,
			&s3.PutObjectInput{
				Bucket: aws.String(Bucket),
				Key:    aws.String(objectKey),
			},
			func(opts *s3.PresignOptions) {
				opts.Expires = 15 * time.Minute
			},
		)

		if err != nil {
			c.JSON(500, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(200, gin.H{
			"uploadUrl": result.URL,
			"objectKey": objectKey,
		})
	})

	// Generate presigned download URL
	r.GET("/download-url/:object", func(c *gin.Context) {

		object := c.Param("object")

		result, err := presignClient.PresignGetObject(
			ctx,
			&s3.GetObjectInput{
				Bucket: aws.String(Bucket),
				Key:    aws.String(object),
			},
			func(opts *s3.PresignOptions) {
				opts.Expires = 30 * time.Minute
			},
		)

		if err != nil {

			c.JSON(500, gin.H{
				"error": err.Error(),
			})

			return
		}

		c.JSON(200, gin.H{
			"url": result.URL,
		})
	})

	log.Printf(
		"🚀 Server berjalan di http://localhost%s",
		ServerPort,
	)

	log.Fatal(
		r.Run(ServerPort),
	)
}
