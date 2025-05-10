package queue

import (
	"context"
	// "encoding/json" // Not directly used in this file anymore, but good to keep if payloads are complex
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MessageQueueService defines the interface for a message queue.
type MessageQueueService interface {
	Publish(ctx context.Context, exchange, routingKey string, body []byte, delayMilliseconds int32) error
	Consume(queueName, consumerTag string, handler func(delivery amqp.Delivery) error) error // Added error return for handler
	Close() error
	DeclareDelayedMessageExchangeAndQueue(exchangeName, queueName, deadLetterExchange, deadLetterRoutingKey string) error
	DeclareExchange(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	DeclareQueue(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	BindQueue(queueName, routingKey, exchangeName string, noWait bool, args amqp.Table) error
	IsInitialized() bool // Added IsInitialized method to the interface
}

// RabbitMQService implements MessageQueueService for RabbitMQ.
type RabbitMQService struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewRabbitMQService creates a new RabbitMQ service.
func NewRabbitMQService(url string) (*RabbitMQService, error) {
	if url == "" {
		log.Println("RabbitMQ URL is empty, RabbitMQ service will be a no-op.")
		return &RabbitMQService{}, nil // Return a no-op service if URL is not configured
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	return &RabbitMQService{conn: conn, channel: ch}, nil
}

// IsInitialized checks if the RabbitMQ connection and channel are established.
func (s *RabbitMQService) IsInitialized() bool {
	return s.conn != nil && s.channel != nil
}

// Publish sends a message to RabbitMQ.
func (s *RabbitMQService) Publish(ctx context.Context, exchange, routingKey string, body []byte, delayMilliseconds int32) error {
	if !s.IsInitialized() {
		log.Println("RabbitMQ channel not initialized. Skipping publish.")
		return nil // No-op if not initialized
	}
	publishing := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		Timestamp:    time.Now(),
		DeliveryMode: amqp.Persistent, // Make messages persistent
	}

	if delayMilliseconds > 0 {
		publishing.Expiration = fmt.Sprintf("%d", delayMilliseconds) // Per-message TTL
	}

	err := s.channel.PublishWithContext(ctx,
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		publishing,
	)
	if err != nil {
		return fmt.Errorf("failed to publish a message: %w", err)
	}
	log.Printf("Published message to exchange '%s', routing key '%s'", exchange, routingKey)
	return nil
}

// DeclareExchange declares a RabbitMQ exchange.
func (s *RabbitMQService) DeclareExchange(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	if !s.IsInitialized() {
		return fmt.Errorf("RabbitMQ channel not initialized")
	}
	return s.channel.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

// DeclareQueue declares a RabbitMQ queue.
func (s *RabbitMQService) DeclareQueue(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	if !s.IsInitialized() {
		return amqp.Queue{}, fmt.Errorf("RabbitMQ channel not initialized")
	}
	return s.channel.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

// BindQueue binds a queue to an exchange.
func (s *RabbitMQService) BindQueue(queueName, routingKey, exchangeName string, noWait bool, args amqp.Table) error {
	if !s.IsInitialized() {
		return fmt.Errorf("RabbitMQ channel not initialized")
	}
	return s.channel.QueueBind(queueName, routingKey, exchangeName, noWait, args)
}

// DeclareDelayedMessageExchangeAndQueue sets up exchanges and queues for delayed messages using DLX.
func (s *RabbitMQService) DeclareDelayedMessageExchangeAndQueue(
	delayExchangeName, delayQueueName, actualExchangeName, actualRoutingKey string) error {
	if !s.IsInitialized() {
		log.Println("RabbitMQ channel not initialized. Skipping DLX declaration.")
		return nil // No-op
	}

	// 1. Declare the "actual" exchange (where messages go after delay)
	err := s.DeclareExchange(actualExchangeName, "direct", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare actual exchange '%s': %w", actualExchangeName, err)
	}

	// 2. Declare the "delay" queue. Messages sit here until TTL expires.
	_, err = s.DeclareQueue(delayQueueName, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    actualExchangeName,
		"x-dead-letter-routing-key": actualRoutingKey,
	})
	if err != nil {
		return fmt.Errorf("failed to declare delay queue '%s': %w", delayQueueName, err)
	}

	// 3. Declare the "delay" exchange (where messages are initially published with TTL)
	err = s.DeclareExchange(delayExchangeName, "direct", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare delay exchange '%s': %w", delayExchangeName, err)
	}

	// 4. Bind the "delay" queue to the "delay" exchange
	// Using the queue name as the binding key for the direct exchange.
	err = s.BindQueue(delayQueueName, delayQueueName, delayExchangeName, false, nil)
	if err != nil {
		return fmt.Errorf("failed to bind delay queue '%s' to delay exchange '%s': %w", delayQueueName, delayExchangeName, err)
	}

	log.Printf("Declared delayed message setup: delayExchange='%s', delayQueue='%s', actualExchange='%s', actualRoutingKey='%s'",
		delayExchangeName, delayQueueName, actualExchangeName, actualRoutingKey)
	return nil
}

// Consume starts consuming messages from a queue.
func (s *RabbitMQService) Consume(queueName, consumerTag string, handler func(delivery amqp.Delivery) error) error {
	if !s.IsInitialized() {
		log.Printf("RabbitMQ channel not initialized. Cannot consume from queue '%s'.", queueName)
		return fmt.Errorf("RabbitMQ channel not initialized")
	}
	msgs, err := s.channel.Consume(
		queueName,
		consumerTag,
		false, // auto-ack (false means manual ack/nack)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer for queue '%s': %w", queueName, err)
	}

	go func() {
		for d := range msgs {
			log.Printf("Received a message on queue '%s', deliveryTag %d", queueName, d.DeliveryTag)
			if err := handler(d); err != nil {
				log.Printf("Error processing message (deliveryTag %d) from queue '%s': %v. Nacking.", d.DeliveryTag, queueName, err)
				// Nack and requeue=false to send to DLX if configured, or discard.
				// Be careful with requeue=true to avoid infinite loops for poison pills.
				if nackErr := d.Nack(false, false); nackErr != nil {
					log.Printf("Error Nacking message (deliveryTag %d): %v", d.DeliveryTag, nackErr)
				}
			} else {
				log.Printf("Successfully processed message (deliveryTag %d) from queue '%s'. Acking.", d.DeliveryTag, queueName)
				if ackErr := d.Ack(false); ackErr != nil {
					log.Printf("Error Acking message (deliveryTag %d): %v", d.DeliveryTag, ackErr)
				}
			}
		}
		log.Printf("RabbitMQ consumer for queue '%s' (tag: '%s') has stopped.", queueName, consumerTag)
	}()

	log.Printf("Registered consumer for queue '%s' with tag '%s'", queueName, consumerTag)
	return nil
}

// Close closes the RabbitMQ connection and channel.
func (s *RabbitMQService) Close() error {
	if s.channel != nil {
		if err := s.channel.Close(); err != nil {
			log.Printf("Error closing RabbitMQ channel: %v", err)
			// Attempt to close connection anyway
		}
		s.channel = nil // Mark as closed
		log.Println("RabbitMQ channel closed.")
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			return fmt.Errorf("failed to close RabbitMQ connection: %w", err)
		}
		s.conn = nil // Mark as closed
		log.Println("RabbitMQ connection closed.")
	}
	return nil
}
