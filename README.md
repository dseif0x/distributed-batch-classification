# Image Processing Microservices System

This project is a containerized microservices system for uploading, storing, and processing images using NATS for interprocess communication. It includes:

* `image-registry`: Handles image uploads
* `image-service`: Serves stored images by ID
* `inference-service`: Handles inference tasks on images
* `metadata-service`: Manages image metadata
* `NATS`: Message broker for communication between services
* `MongoDB`: Metadata storage backend
* `Traefik`: Reverse Proxy to expose all services via a single port

## 🚀 Quick Start

1. **Clone the repository**

```bash
git clone https://github.com/dseif0x/image-system.git
cd image-system
```

2. **Copy the environment file**

```bash
cp .env.example .env
```

Edit `.env` to adjust MongoDB credentials if necessary.

3. **Build and start the system**

```bash
docker-compose up --build
```

This will start all four services along with MongoDB and NATS.

## 📡 Exposed Endpoints

| Method | URL                                 | Description                    |
| ------ | ----------------------------------- | ------------------------------ |
| POST   | `http://localhost:8011/registry/upload`      | Upload a new image             |
| GET    | `http://localhost:8011/metadata/images/list` | List all stored image metadata |
| GET    | `http://localhost:8011/image/images/{id}` | Retrieve an image by its ID    |

## 🔌 Architecture

All services communicate using **NATS JetStream** for event-based messaging. MongoDB is used to store metadata such as image IDs and labels.

Each service is self-contained and follows a single responsibility principle:

* `image-registry` publishes uploaded images to NATS and stores them in an object store
* `image-service` retrieves and serves images from the object store
* `inference-service` listens to uploaded image events and performs inference
* `metadata-service` listens to metadata-related events and stores/retrieves metadata in MongoDB

## 🛠 Services Directory

```
.
├── .env.example
├── .gitignore
├── docker-compose.yml
├── image-registry        # Handles image uploads
├── image-service         # Serves image data
├── inference-service     # Processes images for inference
└── metadata-service      # Handles metadata storage and lookup
```

## 🧪 Development

Each service is independently containerized. You can rebuild or restart a single service using Docker Compose:

```bash
docker-compose up --build metadata-service
```

## 📬 Communication

All internal communication is done through **NATS**, allowing decoupled, scalable messaging between microservices.

## 📖 License

MIT License

