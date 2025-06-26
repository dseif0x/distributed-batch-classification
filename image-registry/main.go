package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	bucketName             = "images"
	startClassifySubject   = "images.classify"
	metaDataServiceSubject = "images.metadata.store"
)

type MetaDataRequest struct {
	ImageID      string `json:"image_id"`
	OriginalName string `json:"original_name,omitempty"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, continuing...")
	}

	// Get NATS URL from environment variables
	natsURL, found := os.LookupEnv("NATS_URL")
	if !found {
		panic("NATS_URL not found in environment variables")
	}

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Drain()

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Failed to get JetStream context: %v", err)
	}

	objectStoreConfig := jetstream.ObjectStoreConfig{
		Bucket: bucketName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create object store bucket (if not exists)
	store, err := js.CreateOrUpdateObjectStore(ctx, objectStoreConfig)
	if err != nil {
		log.Fatalf("Failed to create object store: %v", err)
	}

	http.HandleFunc("/registry/upload", func(w http.ResponseWriter, r *http.Request) {
		// Parse uploaded file
		err := r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			http.Error(w, "Failed to parse multipart form", http.StatusBadRequest)
			return
		}

		file, handler, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "Image is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Generate image ID
		imageID := uuid.New().String()

		// Store image in NATS Object Store
		headers := make(nats.Header)
		headers.Set("Content-Type", handler.Header.Get("Content-Type"))
		uploadInfo, err := store.Put(context.Background(), jetstream.ObjectMeta{Name: imageID, Headers: headers}, file)
		if err != nil {
			http.Error(w, "Failed to store image", http.StatusInternalServerError)
			log.Printf("ObjectStore error: %v", err)
			return
		}

		log.Printf("Stored image %s (%s), size %d bytes", imageID, handler.Filename, uploadInfo.Size)

		meta := MetaDataRequest{
			ImageID:      imageID,
			OriginalName: handler.Filename,
		}
		// Publish metadata request
		jsonstr, err := json.Marshal(&meta)
		_, err = nc.Request(metaDataServiceSubject, jsonstr, time.Second)
		if err != nil {
			http.Error(w, "Failed to send metadata request", http.StatusInternalServerError)
			log.Printf("Failed to send metadata request: %v", err)
			return
		}
		log.Printf("Published message for image %s", imageID)

		// Queue Classification
		_, err = js.PublishAsync(startClassifySubject, []byte(imageID))
		if err != nil {
			http.Error(w, "Failed to publish classification request", http.StatusInternalServerError)
			log.Printf("Failed to publish classification request: %v", err)
			return
		}

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "Image uploaded with ID: %s\n", imageID)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
