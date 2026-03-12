package producer

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/segmentio/kafka-go"
)

// Producer wraps a kafka.Writer for sensor event publishing.
type Producer struct {
	writer *kafka.Writer
}

// New creates a Kafka producer for the given brokers and topic.
func New(brokers, topic string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(strings.Split(brokers, ",")...),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
		},
	}
}

// Publish encodes a SensorEvent as JSON and sends it to Kafka.
func (p *Producer) Publish(ctx context.Context, event models.SensorEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.SensorID),
		Value: data,
	})
}

// Close shuts down the Kafka writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
