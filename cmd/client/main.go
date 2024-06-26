package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/unappendixed/bootdevpubsub/internal/gamelogic"
	"github.com/unappendixed/bootdevpubsub/internal/pubsub"
	"github.com/unappendixed/bootdevpubsub/internal/routing"
)

var logger log.Logger

const logfilepath string = "client.log"

func main() {

	godotenv.Load(".env")
	connstr, found := os.LookupEnv("RABBITMQ_CONN_STRING")
	if !found {
		panic("AMQP connection string not found!")
	}

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
	defer conn.Close()

	input, err := gamelogic.ClientWelcome()

	pauseKey := fmt.Sprintf("%s.%s", routing.PauseKey, input)

	_, _, err = pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilDirect,
		pauseKey,
		routing.PauseKey,
		pubsub.QueueTypeTransient,
	)
	if err != nil {
		panic(err)
	}

	armyKey := fmt.Sprintf("%s.%s", routing.ArmyMovesPrefix, input)

    // Outgoing army moves
	_, _, err = pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilTopic,
		"",
		armyKey,
		pubsub.QueueTypeTransient,
	)

	gamestate := gamelogic.NewGameState(input)

    // Incoming wars
	err = pubsub.SubscribeJSON[gamelogic.RecognitionOfWar](
		conn,
		routing.ExchangePerilTopic,
		fmt.Sprintf("%s.*", routing.WarRecognitionsPrefix),
		"war",
		pubsub.QueueTypeDurable,
		handlerWar(gamestate, conn),
	)

    // Incoming playing state
	err = pubsub.SubscribeJSON[routing.PlayingState](
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey,
		pauseKey,
		pubsub.QueueTypeTransient,
		handlerPause(gamestate),
	)
	if err != nil {
		panic(fmt.Errorf("Failed to subscribe to server pause state: %w", err))
	}

    // Incoming moves
	err = pubsub.SubscribeJSON[gamelogic.ArmyMove](
		conn,
		routing.ExchangePerilTopic,
		fmt.Sprintf("%s.*", routing.ArmyMovesPrefix),
		"",
		pubsub.QueueTypeTransient,
		handlerArmyMove(gamestate, conn, input),
	)
	if err != nil {
		panic(fmt.Errorf("Failed to subscribe to army moves: %w", err))
	}

outer:
	for {
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
			move, err := gamestate.CommandMove(input)
			if err != nil {
				fmt.Println(err)
			}

            ch, err := conn.Channel()
            if err != nil {
				panic(fmt.Errorf("Failed to publish army move: %w", err))
            }
            defer ch.Close()
			err = pubsub.PublishJSON[gamelogic.ArmyMove](
				ch,
				routing.ExchangePerilTopic,
				armyKey,
				move,
			)
			if err != nil {
				panic(fmt.Errorf("Failed to publish army move: %w", err))
			}

			fmt.Println("Move was published to other players.")

		case "help":
			gamelogic.PrintClientHelp()

		case "status":
			gamestate.CommandStatus()

		case "spam":
            if len(input) != 2 {
                fmt.Println("Spam command must specify exactly one argument.")
                continue
            }

            count, err := strconv.Atoi(input[1])
            if err != nil || count <= 0 {
                fmt.Println("Argument must be a positive integer")
                continue
            }

            ch, err := conn.Channel()
            if err != nil {
                fmt.Println(err)
                continue
            }
            defer ch.Close()

            logstr := gamelogic.GetMaliciousLog()

            for i := 0; i < count; i++ {
                pubsub.PublishGob[routing.GameLog](
                    ch,
                    routing.ExchangePerilTopic,
                    fmt.Sprintf("%s.%s", routing.GameLogSlug, gamestate.Player.Username),
                    routing.GameLog{
                        CurrentTime: time.Now(),
                        Message: logstr,
                        Username: gamestate.Player.Username,
                    },
                )
            }

			fmt.Println("Spamming chat...")

		default:
			fmt.Printf("Unkown command: %q\n", input[0])
		}
	}
}

func handlerArmyMove(
	gs *gamelogic.GameState,
	conn *amqp.Connection,
	username string) func(gamelogic.ArmyMove) pubsub.AckType {

	return func(am gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(am)
        fmt.Println("Move received!")

		switch outcome {
		case gamelogic.MoveOutComeSafe:
			return pubsub.AckTypeAck
		case gamelogic.MoveOutcomeMakeWar:
			row := gamelogic.RecognitionOfWar{
				Attacker: am.Player,
				Defender: gs.Player,
			}
			ch, err := conn.Channel()
			defer ch.Close()
			if err != nil {
				fmt.Println(err)
				return pubsub.AckTypeNackRequeue
			}
			err = pubsub.PublishJSON[gamelogic.RecognitionOfWar](
				ch,
				routing.ExchangePerilTopic,
				fmt.Sprintf("%s.%s", routing.WarRecognitionsPrefix, username),
				row,
			)
			if err != nil {
				fmt.Println(err)
			}
			return pubsub.AckTypeAck
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.AckTypeNackDiscard
		default:
			return pubsub.AckTypeAck
		}
	}
}

func handlerWar(gs *gamelogic.GameState, conn *amqp.Connection) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(rw gamelogic.RecognitionOfWar) pubsub.AckType {
        defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(rw)

        gamelog := routing.GameLog{
            CurrentTime: time.Now(),
            Username: gs.Player.Username,
        }

        ch, err := conn.Channel()
        if err != nil {
            return pubsub.AckTypeNackRequeue
        }
        defer ch.Close()

        publish := func(gl routing.GameLog) {
            pubsub.PublishGob[routing.GameLog](
                ch,
                routing.ExchangePerilTopic,
                fmt.Sprintf("%s.%s", routing.GameLogSlug, rw.Attacker.Username),
                gamelog,
            )
        }

		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.AckTypeNackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.AckTypeNackDiscard
		case gamelogic.WarOutcomeOpponentWon:

            gamelog.Message = fmt.Sprintf("%s won a war against %s", winner, loser )
            publish(gamelog)

			return pubsub.AckTypeAck
		case gamelogic.WarOutcomeDraw:

            gamelog.Message = fmt.Sprintf(
                "A war between %s and %s resulted in a draw",
                winner,
                loser,
            )
            publish(gamelog)

			return pubsub.AckTypeAck
		case gamelogic.WarOutcomeYouWon:

            gamelog.Message = fmt.Sprintf("%s won a war against %s", winner, loser )
            publish(gamelog)

			return pubsub.AckTypeAck
		default:
			fmt.Printf("Invalid war outcome: %d", outcome)
			return pubsub.AckTypeNackDiscard
		}
	}
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	return func(ps routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.AckTypeAck
	}
}
