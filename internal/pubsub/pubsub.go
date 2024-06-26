package pubsub

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type QueueType int

const (
	QueueTypeDurable QueueType = iota
	QueueTypeTransient
)

type AckType int

const (
	AckTypeAck AckType = iota
	AckTypeNackRequeue
	AckTypeNackDiscard
)

var InvalidQueueTypeErr error = errors.New("Unknown queue type specified")

func PublishJSON[T any](ch *amqp.Channel, exchange string, key string, val T) error {

    err := publish[T](ch, exchange, key, val, func(t T) ([]byte, error) {
        bytes, err := json.Marshal(val)
        if err != nil {
            return nil, err
        }

        return bytes, nil
    })
    if err != nil {
        return err
    }

    return nil
}

func PublishGob[T any](ch *amqp.Channel, exchange string, key string, val T) error {
    err := publish[T](ch, exchange, key, val, func(t T) ([]byte, error) {
        var buf bytes.Buffer
        encoder := gob.NewEncoder(&buf)
        err := encoder.Encode(val)
        if err != nil {
            return nil, err
        }

        return buf.Bytes(), nil
    })
    if err != nil {
        return err
    }

    return nil
}

func publish[T any](
    ch *amqp.Channel,
    exchange string,
    key string,
    val T,
    marshaller func(T) ([]byte, error),
) error {

    bytes, err := marshaller(val)
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
			ContentType: "application/gob",
			Body:        bytes,
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
	simpleQueueType QueueType,
	handler func(T) AckType,
) error {

    err := subscribe[T](
        conn,
        exchange,
        queueName,
        key,
        simpleQueueType,
        handler,
        func(b []byte) (T, error) {
            var val T
            err := json.Unmarshal(b, &val)
            if err != nil {
                return val, err
            }

            return val, nil
        },
    )

    if err != nil {
        return err
    }

    return nil

}

func SubscribeGob[T any](
	conn *amqp.Connection,
	exchange string,
	key string,
	queueName string,
	simpleQueueType QueueType,
	handler func(T) AckType,
) error {

    err := subscribe[T](
        conn,
        exchange,
        queueName,
        key,
        simpleQueueType,
        handler,
        func(b []byte) (T, error) {
            buf := bytes.NewBuffer(b)
            decoder := gob.NewDecoder(buf)
            var val T
            err := decoder.Decode(&val)
            if err != nil {
                return val, err
            }
            return val, nil
        },

    )
    if err != nil {
        return err
    }

    return nil

}

func subscribe[T any](
    conn *amqp.Connection,
    exchange string,
    queueName string,
    key string,
    simpleQueueType QueueType,
    handler func(T) AckType,
    unmarshaller func([]byte) (T, error),
) error {
	ch, queue, err := DeclareAndBind(
		conn,
		exchange,
		queueName,
		key,
		simpleQueueType,
	)
	if err != nil {
        return err
	}

    ch.Qos(10, 0, false)
    deliveryCh, err := ch.Consume(
        queue.Name,
        "",
        false,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        return err
    }

    go func() {
        for delivery := range deliveryCh {

            val, err := unmarshaller(delivery.Body)
            if err != nil {
                fmt.Println(err)
                delivery.Nack(false, true)
            }

            acktype := handler(val)
            
            switch acktype {
            case AckTypeAck:
                delivery.Ack(false)
            case AckTypeNackRequeue:
                delivery.Nack(false, true)
            case AckTypeNackDiscard:
                delivery.Nack(false, false)
            default:
                delivery.Ack(false)
            }
        }
    }()

    return nil
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange string,
	queueName string,
	key string,
	simpleQueueType QueueType,
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

	switch simpleQueueType {
	case QueueTypeDurable:
		options.durable = true
		options.autoDelete = false
		options.exclusive = false
	case QueueTypeTransient:
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
		amqp.Table{
			"x-dead-letter-exchange": "peril_dlx",
		},
	)
	if err != nil {
		return nil, nil, err
	}

	ch.QueueBind(queueName, key, exchange, false, nil)

	return ch, &queue, nil

}
