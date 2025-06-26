package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	bucketName = "images"
)

func main() {
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

	http.HandleFunc("/image/images/", func(w http.ResponseWriter, r *http.Request) {
		imageId := strings.TrimPrefix(r.URL.Path, "/image/images/")
		info, err := store.GetInfo(context.TODO(), imageId)
		if err != nil {
			log.Printf("could not get image info from object store: %v", err)
			http.Error(w, "Image not found", http.StatusNotFound)
			return
		}

		// Set Content-Type header based on info.Headers.ContentType
		w.Header().Set("Content-Type", info.Headers.Get("Content-Type"))

		imageBytes, err := store.GetBytes(context.TODO(), imageId)
		if err != nil {
			log.Printf("could not get image from object store: %v", err)
			http.Error(w, "Image not found", http.StatusNotFound)
			return
		}

		_, _ = w.Write(imageBytes)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
