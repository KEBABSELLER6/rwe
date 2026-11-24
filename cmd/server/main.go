package main

import (
	"kebabseller6/rwe/internal/handler"
	"log"
	"net/http"
)

func main() {
	router, err := handler.NewRouter()
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(http.ListenAndServe(":8999", router))
}
