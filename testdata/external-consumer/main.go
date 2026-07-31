// Command simon-consumer is a compile-only fixture proving Simon resolves
// and builds as a true external module dependency, using only its public
// import paths.
package main

import (
	"context"
	"fmt"
	"log"

	simon "github.com/LuisKeys/simon"
	"github.com/LuisKeys/simon/model"
)

func main() {
	rt, err := simon.New(simon.WithModel(model.EchoModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main", simon.WithSystemPrompt("You are a local personal assistant."))
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()

	response, err := session.Run(context.Background(), "Hello Simon")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text)
}
