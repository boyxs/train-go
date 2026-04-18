package main

import "log"

func main() {
	repository := InitUserRepository()
	log.Print(repository)
}

//go run .
