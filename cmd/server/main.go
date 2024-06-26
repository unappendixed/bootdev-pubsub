package main

import (
	"fmt"
	"log"
	"os"
    "github.com/joho/godotenv"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/unappendixed/bootdevpubsub/internal/gamelogic"
	"github.com/unappendixed/bootdevpubsub/internal/pubsub"
	"github.com/unappendixed/bootdevpubsub/internal/routing"
)

const logfilepath string = "server.log"

var logger log.Logger

func main() {

    godotenv.Load(".env")

    connstr, found := os.LookupEnv("RABBITMQ_CONN_STRING")
    if !found {
        panic("AMQP connection string not found!")
    }

	logfile, err := os.OpenFile(logfilepath, os.O_CREATE, os.FileMode(0666))
	if err != nil {
		panic(fmt.Errorf("Failed to open logfile %q: %w", logfilepath, err))
	}
	logger = *log.New(logfile, "", log.Ldate|log.Ltime)
	conn, err := amqp.Dial(connstr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		panic(err)
	}
    defer ch.Close()

    err = pubsub.SubscribeGob[routing.GameLog](
        conn,
        routing.ExchangePerilTopic,
        fmt.Sprintf("%s.*", routing.GameLogSlug),
        "game_logs",
        pubsub.QueueTypeDurable,
        func(gl routing.GameLog) pubsub.AckType {
            defer fmt.Print("> ")
            err := gamelogic.WriteLog(gl)
            if err != nil {
                fmt.Println(err)
                return pubsub.AckTypeNackRequeue
            }
            return pubsub.AckTypeAck
        },
    )
    if err != nil {
        panic(err)
    }

    if err != nil {
        logger.Println("Failed to bind to game logs exchange")
    }

	fmt.Printf("Connected to %s\n", connstr)
	gamelogic.PrintServerHelp()

    outer: for {
		input := gamelogic.GetInput()
		if len(input) == 0 {
			continue
		}

        switch input[0] {

        case "pause":
			fmt.Println("Sending pause message...")
			pubsub.PublishJSON(
                ch,
				routing.ExchangePerilDirect,
				routing.PauseKey,
				routing.PlayingState{IsPaused: true},
            )
        case "resume":
            fmt.Println("Sending resume message...")
            pubsub.PublishJSON(
                ch,
                routing.ExchangePerilDirect,
                routing.PauseKey,
                routing.PlayingState{IsPaused: false},
            )
        case "quit":
            fmt.Println("Exiting...")
            break outer
        default:
            fmt.Printf("Unknown command: %q\n", input[0])
		}
    }
}
