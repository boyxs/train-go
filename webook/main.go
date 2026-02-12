package main

func main() {
	server := InitWebServer()
	err := server.Run(":8089")
	if err != nil {
		panic(err)
	}
}
