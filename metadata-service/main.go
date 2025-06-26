package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	streamName                  = "METADATA_EVENTS"
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
		Name:      streamName,
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

	http.HandleFunc("/metadata/images/list", func(w http.ResponseWriter, r *http.Request) {
		res, err := collection.Find(context.TODO(), bson.M{}, options.Find().SetProjection(bson.M{"_id": 0, "image_id": 1, "label": 1}))
		if err != nil {
			log.Printf("Failed to retrieve images: %v", err)
			http.Error(w, fmt.Sprintf("Failed to retrieve images: %v", err), http.StatusInternalServerError)
			return
		}

		var images []map[string]interface{}
		if err := res.All(context.TODO(), &images); err != nil {
			log.Printf("Failed to decode images: %v", err)
			http.Error(w, fmt.Sprintf("Failed to decode images: %v", err), http.StatusInternalServerError)
			return
		}
		// Send the list of images as JSON response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(images); err != nil {
			log.Printf("Failed to encode images: %v", err)
			http.Error(w, fmt.Sprintf("Failed to encode images: %v", err), http.StatusInternalServerError)
			return
		}
		log.Println("List of images sent successfully")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
