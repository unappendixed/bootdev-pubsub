package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/unappendixed/bootdevpubsub/internal/pubsub"
	"github.com/unappendixed/bootdevpubsub/internal/routing"
)

func main() {
    godotenv.Load(".env")
    connstr, found := os.LookupEnv("RABBITMQ_CONN_STRING")
    if !found {
        panic("AMQP connection string not found.")
    }

    conn, err := amqp.Dial(connstr)
    if err != nil {
        panic(fmt.Errorf("Failed to connect to amqp server: %w", err))
    }
    defer conn.Close()

    ch, err := conn.Channel()
    if err != nil {
        panic(fmt.Errorf("Failed to open amqp channel: %w", err))
    }
    defer ch.Close()


    // direct
    err = ch.ExchangeDeclare(
        routing.ExchangePerilDirect,
        amqp.ExchangeDirect,
        false,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        panic(fmt.Errorf("Failed to create direct exchange: %w", err))
    }

    // topic
    err = ch.ExchangeDeclare(
        routing.ExchangePerilTopic,
        amqp.ExchangeTopic,
        true,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        panic(fmt.Errorf("Failed to create topic exchange: %w", err))
    }

    // dead letters
    err = ch.ExchangeDeclare(
        "peril_dlx",
        amqp.ExchangeFanout,
        true,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        panic(fmt.Errorf("Failed to create dead letter exchange: %w", err))
    }

    _, _, err = pubsub.DeclareAndBind(
        conn,
        "peril_dlx",
        "peril_dlq",
        "",
        pubsub.QueueTypeDurable,
    )
    if err != nil {
        panic(fmt.Errorf("Failed to declare/bind dead letter queue: %w", err))
    }

    // Game logs queue

    // pubsub.DeclareAndBind(
    //     conn,
    //     routing.ExchangePerilTopic,
    //     "game_logs",
    //     fmt.Sprintf("%s.*", routing.GameLogSlug),
    //     pubsub.QueueTypeDurable,
    // )

}
