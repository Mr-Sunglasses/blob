# Image Uploader

This is a simple web application for uploading images and getting a public link. It uses Go for the backend, MinIO for object storage, and Docker for containerization.

## Features

-   Image upload through a simple web interface.
-   Generates a unique, shareable link for each uploaded image.
-   Uses MinIO for scalable and reliable object storage.
-   Fully containerized with Docker and Docker Compose for easy deployment.

## Prerequisites

-   Docker
-   Docker Compose

## How to Run

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/gemini/blobs
    cd blobs
    ```

2.  **Configure MinIO credentials:**

    Open the `docker-compose.yml` file and replace the placeholder values for `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD` with your own secure credentials.

3.  **Build and run the application:**

    ```bash
    docker-compose up --build
    ```

4.  **Access the application:**

    Open your web browser and navigate to `http://localhost:8080`.

## How it Works

-   The Go application serves a simple HTML page with an upload form.
-   When an image is uploaded, the application generates a unique filename and uploads it to the configured MinIO bucket.
-   The application then returns a public link to the uploaded image.
-   The `/uploads/:filename` endpoint serves the images directly from the MinIO bucket.

## Project Structure

```
.
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
├── main.go
├── README.md
└── templates
    └── index.html
```
