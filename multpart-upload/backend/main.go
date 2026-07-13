package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	Bucket = "multipart-uploads"
	Region = "us-east-1"

	AccessKey = "minioadmin"
	SecretKey = "minioadmin123"

	Endpoint = "http://localhost:9000"

	ServerPort = ":8080"
)

type InitRequest struct {
	FileName   string `json:"fileName"`
	FileSize   int64  `json:"fileSize"`
	TotalParts int32  `json:"totalParts"`
}

type CompleteRequest struct {
	UploadID  string `json:"uploadId"`
	ObjectKey string `json:"objectKey"`
	Parts     []Part `json:"parts"`
}

type Part struct {
	PartNumber int32  `json:"partNumber"`
	ETag       string `json:"etag"`
}

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
		ExposeHeaders: []string{
			"Content-Length",
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.POST("/multipart/init", func(c *gin.Context) {

		var req InitRequest

		if err := c.ShouldBindJSON(&req); err != nil {

			c.JSON(400, gin.H{
				"error": err.Error(),
			})

			return
		}

		objectKey := fmt.Sprintf(
			"%s-%s",
			uuid.New(),
			req.FileName,
		)

		log.Printf(
			"Start multipart upload: %s\n",
			objectKey,
		)

		createOutput, err := client.CreateMultipartUpload(
			c,
			&s3.CreateMultipartUploadInput{
				Bucket: aws.String(Bucket),
				Key:    aws.String(objectKey),

				ContentType: aws.String(
					"application/octet-stream",
				),
			},
		)

		if err != nil {

			log.Println("Create multipart error:", err)

			c.JSON(500, gin.H{
				"error": err.Error(),
			})

			return
		}

		type UploadPart struct {
			PartNumber int32  `json:"partNumber"`
			UploadURL  string `json:"uploadUrl"`
		}

		var urls []UploadPart

		for i := int32(1); i <= req.TotalParts; i++ {

			presigned, err := presignClient.PresignUploadPart(
				c,
				&s3.UploadPartInput{
					Bucket:     aws.String(Bucket),
					Key:        aws.String(objectKey),
					UploadId:   createOutput.UploadId,
					PartNumber: aws.Int32(i),
				},
				s3.WithPresignExpires(
					15*time.Minute,
				),
			)

			if err != nil {

				c.JSON(500, gin.H{
					"error": err.Error(),
				})

				return
			}

			urls = append(
				urls,
				UploadPart{
					PartNumber: i,
					UploadURL:  presigned.URL,
				},
			)

		}

		c.JSON(http.StatusOK, gin.H{

			"uploadId": createOutput.UploadId,

			"objectKey": objectKey,

			"parts": urls,
		})

	})

	r.POST("/multipart/complete", func(c *gin.Context) {

		var req CompleteRequest

		if err := c.ShouldBindJSON(&req); err != nil {

			c.JSON(400, gin.H{
				"error": err.Error(),
			})

			return
		}

		sort.Slice(
			req.Parts,
			func(i, j int) bool {
				return req.Parts[i].PartNumber <
					req.Parts[j].PartNumber
			},
		)

		var completed []types.CompletedPart

		for _, p := range req.Parts {

			completed = append(
				completed,
				types.CompletedPart{
					ETag: aws.String(p.ETag),

					PartNumber: aws.Int32(
						p.PartNumber,
					),
				},
			)

		}

		_, err := client.CompleteMultipartUpload(
			c,
			&s3.CompleteMultipartUploadInput{

				Bucket: aws.String(Bucket),

				Key: aws.String(req.ObjectKey),

				UploadId: aws.String(req.UploadID),

				MultipartUpload: &types.CompletedMultipartUpload{
					Parts: completed,
				},
			},
		)

		if err != nil {

			log.Println(
				"Complete upload error:",
				err,
			)

			c.JSON(500, gin.H{
				"error": err.Error(),
			})

			return
		}

		getObject, err := presignClient.PresignGetObject(
			c,
			&s3.GetObjectInput{
				Bucket: aws.String(Bucket),
				Key:    aws.String(req.ObjectKey),

				// tampilkan di browser, bukan download
				ResponseContentDisposition: aws.String(
					"inline",
				),
			},
			s3.WithPresignExpires(
				30*time.Minute,
			),
		)

		if err != nil {

			c.JSON(500, gin.H{
				"error": err.Error(),
			})

			return
		}

		c.JSON(200, gin.H{

			"downloadUrl": getObject.URL,
		})

	})

	log.Println("==============================")
	log.Printf("API Server running on port %s\n", ServerPort)
	log.Printf("MinIO Endpoint: %s\n", Endpoint)
	log.Printf("Bucket: %s\n", Bucket)
	log.Println("==============================")

	if err := r.Run(ServerPort); err != nil {

		log.Fatal(
			"Server stopped:",
			err,
		)

	}

}
