package pubsub

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

type QueueType int
const (
	Durable QueueType = iota
	Transient
)

func PublishJSON[T any](ch *amqp.Channel, exchange string, key string, val T) error {
    bytes, err := json.Marshal(val)
    if err != nil {
        return err
    }

    err = ch.PublishWithContext(
        context.Background(),
        exchange,
        key,
        false,
        false,
        amqp.Publishing{
            ContentType: "application/json",
            Body: bytes,
        },
    )
    if err != nil {
        return err
    }

    return nil

}

func SubscribeJSON[T any](
    conn *amqp.Connection,
    exchange string,
    key string,
    queueName string,
    singleQueueType QueueType,
    handler func(T),
) {

}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange string,
	queueName string,
	key string,
	singleQueueType QueueType,
) (*amqp.Channel, *amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	options := struct {
		durable    bool
		autoDelete bool
		exclusive  bool
	}{}

	switch singleQueueType {
	case Durable:
		options.durable = true
		options.autoDelete = false
		options.exclusive = false
	case Transient:
		options.durable = false
		options.autoDelete = true
		options.exclusive = true
	default:
		return nil, nil, InvalidQueueTypeErr
	}

	queue, err := ch.QueueDeclare(
		queueName,
		options.durable,
		options.autoDelete,
		options.exclusive,
		false,
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	ch.QueueBind(queueName, key, exchange, false, nil)

	return ch, &queue, nil

}
