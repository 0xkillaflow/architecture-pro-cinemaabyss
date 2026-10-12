package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Топики для каждого типа событий
var topics = map[string]string{
	"movie":   "movie-events",
	"user":    "user-events",
	"payment": "payment-events",
}

type Event struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type EventResponse struct {
	Status    string `json:"status"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	Event     Event  `json:"event"`
}

var producer *kgo.Client

func main() {
	port := getEnv("PORT", "8082")
	brokers := strings.Split(getEnv("KAFKA_BROKERS", "kafka:9092"), ",")

	var err error
	producer, err = kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		log.Fatalf("producer: %v", err)
	}
	defer producer.Close()

	go consume(brokers)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"status": true})
	})
	mux.HandleFunc("POST /api/events/{type}", handleEvent)

	log.Printf("Events service started on port %s, brokers: %v", port, brokers)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// Принимает событие по API и отправляет его в нужный топик Kafka
func handleEvent(w http.ResponseWriter, r *http.Request) {
	eventType := r.PathValue("type")
	topic, ok := topics[eventType]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown event type"})
		return
	}

	var payload json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	event := Event{
		ID:        fmt.Sprintf("%s-%d", eventType, time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
	value, _ := json.Marshal(event)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rec, err := producer.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte(event.ID), Value: value}).First()
	if err != nil {
		log.Printf("produce to %s failed: %v", topic, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("Produced event %s to %s [partition %d, offset %d]", event.ID, topic, rec.Partition, rec.Offset)
	writeJSON(w, http.StatusCreated, EventResponse{
		Status:    "success",
		Partition: rec.Partition,
		Offset:    rec.Offset,
		Event:     event,
	})
}

// Читает события из всех топиков и пишет их в лог
func consume(brokers []string) {
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup("events-service"),
		kgo.ConsumeTopics("movie-events", "user-events", "payment-events"),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		log.Fatalf("consumer: %v", err)
	}
	defer consumer.Close()

	for {
		fetches := consumer.PollFetches(context.Background())
		fetches.EachError(func(topic string, partition int32, err error) {
			log.Printf("consume error %s[%d]: %v", topic, partition, err)
		})
		fetches.EachRecord(func(rec *kgo.Record) {
			log.Printf("Consumed event from %s [partition %d, offset %d]: %s", rec.Topic, rec.Partition, rec.Offset, rec.Value)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
