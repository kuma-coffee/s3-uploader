package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
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
	Bucket = "resumable-uploads"
	Region = "us-east-1"

	Endpoint = "http://localhost:9000"

	AccessKey = "minioadmin"
	SecretKey = "minioadmin123"

	ServerPort = ":8080"
)

type UploadSession struct {
	UploadID  string
	ObjectKey string
	FileName  string
}

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

/*
========================================================

DEMO ONLY

Pada production JANGAN gunakan map.

Gunakan database.

Contoh table:

upload_sessions

id
user_id
filename
object_key
upload_id
status
created_at

========================================================
*/

var (
	sessionStore = map[string]UploadSession{}
	mutex        sync.Mutex
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
		ExposeHeaders: []string{
			"Content-Length",
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	/*
		==================================================

		Start Upload

		==================================================
	*/

	r.POST("/upload/start", func(c *gin.Context) {

		startTime := time.Now()

		log.Println("[UPLOAD START] Request masuk")

		var req InitRequest

		if err := c.ShouldBindJSON(&req); err != nil {

			log.Printf(
				"[UPLOAD START][ERROR] Invalid request: %v",
				err,
			)

			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		log.Printf(
			"[UPLOAD START] filename=%s totalParts=%d",
			req.FileName,
			req.TotalParts,
		)

		mutex.Lock()
		session, exists := sessionStore[req.FileName]
		mutex.Unlock()

		/*
			==================================================

			Jika upload belum ada

			Create Multipart Upload

			==================================================
		*/

		if !exists {

			log.Printf(
				"[UPLOAD START] Session tidak ditemukan, membuat multipart upload baru. filename=%s",
				req.FileName,
			)

			objectKey := uuid.New().String() + "-" + req.FileName

			createOutput, err := client.CreateMultipartUpload(
				context.Background(),
				&s3.CreateMultipartUploadInput{
					Bucket: aws.String(Bucket),
					Key:    aws.String(objectKey),
				},
			)

			if err != nil {

				log.Printf(
					"[UPLOAD START][ERROR] CreateMultipartUpload gagal filename=%s error=%v",
					req.FileName,
					err,
				)

				c.JSON(500, gin.H{"error": err.Error()})
				return
			}

			session = UploadSession{
				UploadID:  *createOutput.UploadId,
				ObjectKey: objectKey,
				FileName:  req.FileName,
			}

			log.Printf(
				"[UPLOAD START] Multipart created uploadId=%s objectKey=%s",
				session.UploadID,
				session.ObjectKey,
			)

			mutex.Lock()
			sessionStore[req.FileName] = session
			mutex.Unlock()

			/*
				PRODUCTION

				Simpan ke database

				user_id
				filename
				object_key
				upload_id
				status=UPLOADING
			*/

		} else {

			log.Printf(
				"[UPLOAD START] Resume upload ditemukan uploadId=%s objectKey=%s",
				session.UploadID,
				session.ObjectKey,
			)

		}

		/*
			==================================================

			List uploaded parts

			Inilah yang membuat upload bisa RESUME

			==================================================
		*/

		/*
			==================================================

			List uploaded parts

			Meminta MinIO/S3 daftar part yang sudah berhasil
			di-upload pada multipart upload tertentu.

			Informasi ini digunakan agar upload dapat RESUME.

			Contoh:

			Part 1 ✅
			Part 2 ✅
			Part 3 ❌

			Frontend hanya perlu upload Part 3.

			==================================================
		*/
		partOutput, err := client.ListParts(
			context.Background(),
			&s3.ListPartsInput{
				Bucket:   aws.String(Bucket),
				Key:      aws.String(session.ObjectKey),
				UploadId: aws.String(session.UploadID),
			},
		)

		if err != nil {

			log.Printf(
				"[UPLOAD START][ERROR] ListParts gagal uploadId=%s error=%v",
				session.UploadID,
				err,
			)

			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		uploaded := []int32{}
		//  Cek part yang berhasil di upload dan nantinya akan dikembalikan ke FE
		for _, p := range partOutput.Parts {

			uploaded = append(
				uploaded,
				*p.PartNumber,
			)

		}

		log.Printf(
			"[UPLOAD START] Uploaded parts ditemukan=%v",
			uploaded,
		)

		type UploadURL struct {
			PartNumber int32  `json:"partNumber"`
			UploadURL  string `json:"uploadUrl"`
		}

		var urls []UploadURL

		/*
			==================================================

			Boleh generate semua URL.

			Frontend akan SKIP part
			yang sudah ada.

			==================================================
		*/

		for i := int32(1); i <= req.TotalParts; i++ {

			url, err := presignClient.PresignUploadPart(
				context.Background(),
				&s3.UploadPartInput{
					Bucket:     aws.String(Bucket),
					Key:        aws.String(session.ObjectKey),
					UploadId:   aws.String(session.UploadID),
					PartNumber: aws.Int32(i),
				},
				s3.WithPresignExpires(15*time.Minute),
			)

			if err != nil {

				log.Printf(
					"[UPLOAD START][ERROR] PresignUploadPart gagal part=%d error=%v",
					i,
					err,
				)

				c.JSON(500, gin.H{"error": err.Error()})
				return
			}

			urls = append(
				urls,
				UploadURL{
					PartNumber: i,
					UploadURL:  url.URL,
				},
			)

		}

		log.Printf(
			"[UPLOAD START] Presigned URLs generated=%d duration=%s",
			len(urls),
			time.Since(startTime),
		)

		c.JSON(200, gin.H{
			"uploadId":      session.UploadID,
			"objectKey":     session.ObjectKey,
			"uploadedParts": uploaded,
			"parts":         urls,
		})

	})

	r.POST("/upload/complete", func(c *gin.Context) {

		startTime := time.Now()

		log.Println("[UPLOAD COMPLETE] Request masuk")

		var req CompleteRequest

		if err := c.ShouldBindJSON(&req); err != nil {

			log.Printf(
				"[UPLOAD COMPLETE][ERROR] Invalid request error=%v",
				err,
			)

			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		log.Printf(
			"[UPLOAD COMPLETE] objectKey=%s uploadId=%s parts=%d",
			req.ObjectKey,
			req.UploadID,
			len(req.Parts),
		)

		sort.Slice(req.Parts, func(i, j int) bool {
			return req.Parts[i].PartNumber < req.Parts[j].PartNumber
		})

		var completed []types.CompletedPart

		for _, p := range req.Parts {

			completed = append(
				completed,
				types.CompletedPart{
					ETag:       aws.String(p.ETag),
					PartNumber: aws.Int32(p.PartNumber),
				},
			)

		}

		_, err := client.CompleteMultipartUpload(
			context.Background(),
			&s3.CompleteMultipartUploadInput{
				Bucket:   aws.String(Bucket),
				Key:      aws.String(req.ObjectKey),
				UploadId: aws.String(req.UploadID),
				MultipartUpload: &types.CompletedMultipartUpload{
					Parts: completed,
				},
			},
		)

		if err != nil {

			log.Printf(
				"[UPLOAD COMPLETE][ERROR] CompleteMultipartUpload gagal uploadId=%s error=%v",
				req.UploadID,
				err,
			)

			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		log.Printf(
			"[UPLOAD COMPLETE] Multipart selesai objectKey=%s",
			req.ObjectKey,
		)

		getObject, err := presignClient.PresignGetObject(
			context.Background(),
			&s3.GetObjectInput{
				Bucket: aws.String(Bucket),
				Key:    aws.String(req.ObjectKey),
			},
			s3.WithPresignExpires(30*time.Minute),
		)

		if err != nil {

			log.Printf(
				"[UPLOAD COMPLETE][ERROR] PresignGetObject gagal objectKey=%s error=%v",
				req.ObjectKey,
				err,
			)

			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		/*
			PRODUCTION

			Update status upload

			COMPLETED

			dan simpan object URL.

			Lalu hapus upload session
			atau tandai selesai.

		*/

		mutex.Lock()
		delete(sessionStore, req.ObjectKey) // demo cleanup
		mutex.Unlock()

		log.Printf(
			"[UPLOAD COMPLETE] Session cleanup objectKey=%s duration=%s",
			req.ObjectKey,
			time.Since(startTime),
		)

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
