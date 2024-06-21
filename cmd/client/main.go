package main

import (
	"fmt"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/unappendixed/bootdevpubsub/internal/gamelogic"
	"github.com/unappendixed/bootdevpubsub/internal/routing"
    "github.com/unappendixed/bootdevpubsub/internal/pubsub"
)

const connstr string = "amqp://guest:guest@localhost:5672/"


var logger log.Logger


const logfilepath string = "client.log"

func main() {
	logfile, err := os.OpenFile(logfilepath, os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	logger = *log.New(logfile, "", log.Ldate|log.Ltime)
	fmt.Println("Starting Peril client...")

	conn, err := amqp.Dial(connstr)
	if err != nil {
		panic(err)
	}

	input, err := gamelogic.ClientWelcome()

	name := fmt.Sprintf("%s.%s", routing.PauseKey, input)

	_, _, err = pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilDirect,
		name,
		routing.PauseKey,
		pubsub.Transient,
	)
	if err != nil {
		panic(err)
	}

	gamestate := gamelogic.NewGameState(input)

outer: for {
		input := gamelogic.GetInput()

		if len(input) == 0 {
			continue
		}

		switch input[0] {
		case "quit":
			gamelogic.PrintQuit()
			break outer
		case "spawn":
			if len(input) != 3 {
				fmt.Println("Spawn command must specify exactly two args")
				continue
			}

			gamestate.CommandSpawn(input)

		case "move":
			if len(input) != 3 {
				fmt.Println("Move command must specify exactly two args")
				continue
			}

			gamestate.CommandMove(input)

		case "help":
			gamelogic.PrintClientHelp()

		case "status":
			gamestate.CommandStatus()

		case "spam":
			fmt.Println("Spamming not implemented yet!")

		default:
			fmt.Printf("Unkown command: %q\n", input[0])
		}
	}
}

