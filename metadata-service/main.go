package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	metaDataServiceSubject      = "images.metadata.store"
	newLabelSubject             = "images.metadata.label"
	mongoDatabase               = "images"
	mongoCollection             = "metadata"
	newMetaDataConsumerName     = "new-meta-data-consumer"
	newLabelSubjectConsumerName = "new-label-consumer"
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

	// Get MONGO URL from environment variables
	mongoURL, found := os.LookupEnv("MONGO_URL")
	if !found {
		panic("MONGO_URL not found in environment variables")
	}

	// Get NATS URL from environment variables
	natsURL, found := os.LookupEnv("NATS_URL")
	if !found {
		panic("NATS_URL not found in environment variables")
	}

	// Setup Mongo client
	ctx := context.Background()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	log.Println("Connected to MongoDB:", mongoURL)

	collection := mongoClient.Database(mongoDatabase).Collection(mongoCollection)

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

	streamConfig := jetstream.StreamConfig{
		Name:      "METADATA_EVENTS",
		Subjects:  []string{metaDataServiceSubject, newLabelSubject},
		Retention: jetstream.WorkQueuePolicy,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := jsm.CreateOrUpdateStream(ctx, streamConfig)
	if err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		log.Fatalf("could not create stream: %v", err)
	}

	newMetaDataConsumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       newMetaDataConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy, // Acknowledge each message explicitly
		DeliverPolicy: jetstream.DeliverAllPolicy,  // Deliver all available messages
		FilterSubject: metaDataServiceSubject,
	})
	if err != nil {
		log.Fatalf("could not create newMetaDataConsumer: %v", err)
	}

	_, err = newMetaDataConsumer.Consume(func(msg jetstream.Msg) {
		var imageData map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &imageData); err != nil {
			log.Printf("Could not unmarshal message data: %v", err)
			return
		}
		imageID, ok := imageData["image_id"].(string)
		if !ok || imageID == "" {
			log.Printf("Received message without valid image_id: %v", imageData)
			return
		}

		_, err := collection.InsertOne(context.TODO(), imageData)
		if err != nil {
			log.Printf("Failed to insert metadata into MongoDB: %v", err)
			return
		}
		log.Printf("Stored metadata for image: %s", imageID)
		_ = msg.Ack()
	})
	if err != nil {
		log.Fatalf("could not subscribe to email newMetaDataConsumer: %v", err)
	}

	newLabelSubjectConsumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       newLabelSubjectConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy, // Acknowledge each message explicitly
		DeliverPolicy: jetstream.DeliverAllPolicy,  // Deliver all available messages
		FilterSubject: newLabelSubject,
	})
	if err != nil {
		log.Fatalf("could not create newMetaDataConsumer: %v", err)
	}

	_, err = newLabelSubjectConsumer.Consume(func(msg jetstream.Msg) {
		var newLabelMsg NewLabelMessage
		if err := json.Unmarshal(msg.Data(), &newLabelMsg); err != nil {
			log.Printf("Could not unmarshal label message: %v", err)
			return
		}
		if newLabelMsg.ImageID == "" || newLabelMsg.Label == "" {
			log.Printf("Invalid label message: %v", newLabelMsg)
			return
		}

		filter := bson.M{"image_id": newLabelMsg.ImageID}
		update := bson.M{"$set": bson.M{"label": newLabelMsg.Label}}

		res, err := collection.UpdateOne(context.TODO(), filter, update)
		if err != nil {
			log.Printf("Failed to update label in MongoDB: %v", err)
			return
		}
		if res.MatchedCount == 0 {
			log.Printf("No document found with image_id %s", newLabelMsg.ImageID)
			return
		}

		log.Printf("Updated label for image: %s", newLabelMsg.ImageID)
		_ = msg.Ack()
	})
	if err != nil {
		log.Fatalf("could not subscribe to email newLabelSubjectConsumer: %v", err)
	}

	// Keep the connection alive
	select {}
}
