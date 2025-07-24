package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nfnt/resize"
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

	// Process and compress the image
	processedData, contentType, err := processImage(file, header)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("process image err: %s", err.Error()))
		return
	}

	// Create a unique filename
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), filepath.Ext(header.Filename))
	bucketName := os.Getenv("MINIO_BUCKET")

	// Upload the processed file to MinIO
	_, err = minioClient.PutObject(context.Background(), bucketName, filename, bytes.NewReader(processedData), int64(len(processedData)), minio.PutObjectOptions{
		ContentType: contentType,
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

func processImage(file io.Reader, header *multipart.FileHeader) ([]byte, string, error) {
	// Decode the image
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, "", err
	}

	// Resize if too large (max 1920px width/height)
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	
	if width > 1920 || height > 1920 {
		if width > height {
			img = resize.Resize(1920, 0, img, resize.Lanczos3)
		} else {
			img = resize.Resize(0, 1920, img, resize.Lanczos3)
		}
	}

	// Compress and encode the image
	var buf bytes.Buffer
	var contentType string
	
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		contentType = "image/jpeg"
	case "png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err = encoder.Encode(&buf, img)
		contentType = "image/png"
	default:
		// Default to JPEG for other formats
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
		contentType = "image/jpeg"
	}

	if err != nil {
		return nil, "", err
	}

	return buf.Bytes(), contentType, nil
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

	// Get object stat to set the content type and caching headers
	stat, err := object.Stat()
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to get image stats")
		return
	}

	// Set caching headers for better performance
	c.Writer.Header().Set("Content-Type", stat.ContentType)
	c.Writer.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year
	c.Writer.Header().Set("ETag", stat.ETag)
	c.Writer.Header().Set("Last-Modified", stat.LastModified.UTC().Format(http.TimeFormat))
	
	// Check if client has cached version
	if match := c.GetHeader("If-None-Match"); match != "" && match == stat.ETag {
		c.Status(http.StatusNotModified)
		return
	}
	
	if modifiedSince := c.GetHeader("If-Modified-Since"); modifiedSince != "" {
		if t, err := time.Parse(http.TimeFormat, modifiedSince); err == nil && !stat.LastModified.After(t) {
			c.Status(http.StatusNotModified)
			return
		}
	}

	io.Copy(c.Writer, object)
}
