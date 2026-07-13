package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	const (
		endpoint  = "localhost:9000"
		accessKey = "minioadmin"
		secretKey = "minioadmin123"
		bucket    = "uploads"
		port      = "8080"
	)

	// Connect MinIO
	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(
			accessKey,
			secretKey,
			"",
		),
		Secure: false,
	})

	if err != nil {
		log.Fatalf("❌ Gagal membuat client MinIO: %v", err)
	}

	// Test koneksi MinIO
	ctx := context.Background()

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		log.Fatalf("❌ Tidak bisa connect ke MinIO: %v", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})

		if err != nil {
			log.Fatalf("❌ Gagal membuat bucket %s: %v", bucket, err)
		}

		log.Printf("✅ Bucket '%s' berhasil dibuat", bucket)
	} else {
		log.Printf("✅ Bucket '%s' sudah tersedia", bucket)
	}

	log.Println("✅ Berhasil connect ke MinIO")
	log.Printf("📦 Bucket : %s\n", bucket)
	log.Printf("🌐 MinIO  : http://%s\n", endpoint)

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
	
	r.POST("/upload-url", func(c *gin.Context) {

		fileName := c.Query("filename")
		if fileName == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "filename wajib diisi",
			})
			return
		}

		objectName := fmt.Sprintf("%s-%s", uuid.New().String(), fileName)

		url, err := client.PresignedPutObject(
			ctx,
			bucket,
			objectName,
			15*time.Minute,
		)

		if err != nil {
			log.Printf("PresignedPutObject error: %v", err)

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"uploadUrl": url.String(),
			"objectKey": objectName,
		})
	})

	r.GET("/download-url/:object", func(c *gin.Context) {

		object := c.Param("object")

		url, err := client.PresignedGetObject(
			ctx,
			bucket,
			object,
			30*time.Minute,
			nil,
		)

		if err != nil {
			log.Printf("PresignedGetObject error: %v", err)

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"url": url.String(),
		})
	})

	log.Printf("🚀 Server berjalan di http://localhost:%s\n", port)

	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
