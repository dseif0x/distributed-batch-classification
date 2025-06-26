package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	bucketName           = "images"
	streamName           = "INFERENCE_EVENTS"
	startClassifySubject = "images.classify"
	newLabelSubject      = "images.metadata.label"
	startClassifyName    = "new-label-consumer"
)

type NewLabelMessage struct {
	ImageID string `json:"image_id"`
	Label   string `json:"label"`
}

func main() {
	// Load environment variables from .env file if present
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
		log.Fatalf("could not connect to NATS: %v", err)
	}
	defer nc.Close()
	log.Println("Connected to NATS:", natsURL)

	jsm, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("could not connect to JetStream: %v", err)
	}

	objectStoreConfig := jetstream.ObjectStoreConfig{
		Bucket: bucketName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Create object store bucket (if not exists)
	store, err := jsm.CreateOrUpdateObjectStore(ctx, objectStoreConfig)
	if err != nil {
		log.Fatalf("Failed to create object store: %v", err)
	}

	streamConfig := jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{startClassifySubject},
		Retention: jetstream.WorkQueuePolicy,
	}

	defer cancel()

	stream, err := jsm.CreateOrUpdateStream(ctx, streamConfig)
	if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		log.Fatalf("could not create stream: %v", err)
	}

	startClassifyConsumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       startClassifyName,
		AckPolicy:     jetstream.AckExplicitPolicy, // Acknowledge each message explicitly
		DeliverPolicy: jetstream.DeliverAllPolicy,  // Deliver all available messages
		AckWait:       5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("could not create startClassifyConsumer: %v", err)
	}

	_, err = startClassifyConsumer.Consume(func(msg jetstream.Msg) {
		imageId := string(msg.Data())
		imageBytes, err := store.GetBytes(context.TODO(), imageId)
		if err != nil {
			log.Printf("could not get image from object store: %v", err)
			return
		}
		log.Printf("Received image for classification: %s, size: %d bytes", imageId, len(imageBytes))

		// TODO Classify the image! Currently we just simulate the classification

		newLabelMsg := NewLabelMessage{
			ImageID: imageId,
			Label:   strconv.FormatInt(rand.Int63(), 10),
		}

		newLabelData, err := json.Marshal(newLabelMsg)
		if err != nil {
			log.Printf("could not marshal new label message: %v", err)
			return
		}
		_, err = jsm.PublishAsync(newLabelSubject, newLabelData)

		_ = msg.Ack()
	})
	if err != nil {
		log.Fatalf("could not subscribe to email startClassifyConsumer: %v", err)
	}

	// Keep the connection alive
	select {}
}
