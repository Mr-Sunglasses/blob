package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var minioClient *minio.Client

func main() {
	// MinIO client initialization
	endpoint := os.Getenv("MINIO_ENDPOINT")
	accessKeyID := os.Getenv("MINIO_ACCESS_KEY")
	secretAccessKey := os.Getenv("MINIO_SECRET_KEY")
	useSSLStr := os.Getenv("MINIO_USESSL")
	useSSL, err := strconv.ParseBool(useSSLStr)
	if err != nil {
		useSSL = false
	}

	for i := 0; i < 10; i++ {
		minioClient, err = minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
			Secure: useSSL,
		})
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, err = minioClient.ListBuckets(ctx)
			cancel() // Cancel the context immediately after use
			if err == nil {
				log.Println("Successfully connected to MinIO.")
				break
			}
		}
		log.Printf("Failed to connect to MinIO (attempt %d/10), retrying in 3 seconds: %v", i+1, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to MinIO after multiple retries: %v", err)
	}

	// Create bucket if it doesn't exist
	bucketName := os.Getenv("MINIO_BUCKET")
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		log.Fatalln(err)
	}
	if !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("Successfully created bucket %s\n", bucketName)
	}

	// Gin router setup
	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	router.POST("/upload", handleUpload)
	router.GET("/uploads/:filename", handleServeImage)

	router.Run(":8080")
}

func handleUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("get form err: %s", err.Error()))
		return
	}
	defer file.Close()

	// Create a unique filename
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), filepath.Ext(header.Filename))
	bucketName := os.Getenv("MINIO_BUCKET")

	// Upload the file to MinIO
	_, err = minioClient.PutObject(context.Background(), bucketName, filename, file, header.Size, minio.PutObjectOptions{
		ContentType: header.Header.Get("Content-Type"),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("upload file err: %s", err.Error()))
		return
	}

	// Return the public link
	c.JSON(http.StatusOK, gin.H{
		"url": fmt.Sprintf("/uploads/%s", filename),
	})
}

func handleServeImage(c *gin.Context) {
	filename := c.Param("filename")
	bucketName := os.Getenv("MINIO_BUCKET")

	object, err := minioClient.GetObject(context.Background(), bucketName, filename, minio.GetObjectOptions{})
	if err != nil {
		c.String(http.StatusNotFound, "Image not found")
		return
	}
	defer object.Close()

	// Get object stat to set the content type
	stat, err := object.Stat()
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to get image stats")
		return
	}

	c.Writer.Header().Set("Content-Type", stat.ContentType)
	io.Copy(c.Writer, object)
}
