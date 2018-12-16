package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/soveran/redisurl"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("There is an error in the argument")
		os.Exit(1)
	}
	userName := os.Args[1]

	conn, err := redisurl.ConnectToURL("redis://localhost:6379")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	userKey := "online." + userName
	val, err := conn.Do("SET", userKey, userName, "NX", "EX", "120")
	if err != nil {
		fmt.Println("Do error", err)
		os.Exit(1)
	}
	if val == nil {
		fmt.Println("User already online")
		os.Exit(1)
	}

	val, err = conn.Do("SADD", "users", userName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if val == nil {
		fmt.Println("User already online")
		os.Exit(1)
	}

	tickerChan := time.NewTicker(time.Second * 60).C

	subChan := make(chan string)
	go func() {
		subConn, err := redisurl.ConnectToURL("redis://localhost:6379")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer subConn.Close()

		psc := redis.PubSubConn{Conn: subConn}
		psc.Subscribe("messages")
		for {
			switch v := psc.Receive().(type) {
			case redis.Message:
				subChan <- string(v.Data)
			case redis.Subscription:
				break
			case error:
				return
			}
		}
	}()

	sayChan := make(chan string)
	go func() {
		prompt := userName + ">"
		bio := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(prompt)
			line, _, err := bio.ReadLine()
			if err != nil {
				fmt.Println(err)
				sayChan <- "/exit"
				return
			}
			sayChan <- string(line)
		}
	}()

	conn.Do("PUBLISH", "messages", userName+" has joined")

	chatExit := false

	for !chatExit {
		select {
		case msg := <-subChan:
			fmt.Println(msg)
		case <-tickerChan:
			val, err = conn.Do("SET", userKey, userName, "XX", "EX", "120")
			if err != nil || val == nil {
				fmt.Println("Heartbeat set failed")
				chatExit = true
			}
		case line := <-sayChan:
			if line == "/exit" {
				chatExit = true
			} else if line == "/who" {
				names, _ := redis.Strings(conn.Do("SMEMBERS", "users"))
				for _, name := range names {
					fmt.Println(name)
				}
			} else {
				conn.Do("PUBLISH", "messages", userName+":"+line)
			}
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	conn.Do("DEL", userKey)
	conn.Do("SREM", "users", userName)
	conn.Do("PUBLISH", "messages", userName+" has left")
}
