package main

import (
	"fmt"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/unappendixed/bootdevpubsub/internal/gamelogic"
	"github.com/unappendixed/bootdevpubsub/internal/pubsub"
	"github.com/unappendixed/bootdevpubsub/internal/routing"
)

const connstr string = "amqp://guest:guest@localhost:5672/"
const logfilepath string = "server.log"

var logger log.Logger

func main() {
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

    _, err = ConnectLogs(ch)
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

func ConnectLogs(ch *amqp.Channel) (*amqp.Queue, error) {
    queue, err := ch.QueueDeclare("game_logs", true, false, false, false, nil)
    if err != nil {
        return nil, err
    }

    err = ch.QueueBind(routing.GameLogSlug, routing.GameLogSlug + ".*", routing.ExchangePerilTopic, false, nil)
    if err != nil {
        return nil, err
    }

    return &queue, nil
}
