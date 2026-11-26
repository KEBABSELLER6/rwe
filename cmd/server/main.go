package main

import (
	"context"
	"kebabseller6/rwe/internal/handler"
	"log"
	"net/http"
)

func main() {
	ctx := context.Background()
	router, err := handler.NewRouter(ctx)
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(http.ListenAndServe(":8999", router))
}
