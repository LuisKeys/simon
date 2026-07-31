// Command public_memory demonstrates attaching persistent memory to a
// Session via a MemoryFactory, and shows history surviving across
// multiple Run calls on the same session.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"simon-go/memory"
	"simon-go/model"
	"simon-go/simon"
)

func main() {
	path := "public_memory_example.json"

	// A MemoryFactory builds a Memory implementation per session ID; here
	// each session gets its own JSON file on disk so history persists
	// across process restarts, not just across Run calls in this process.
	factory := memory.FactoryFunc(func(_ context.Context, sessionID string) (memory.Memory, error) {
		return memory.NewJSONFile(sessionID + "-" + path), nil
	})

	rt, err := simon.New(simon.WithModel(model.EchoModel{}), simon.WithMemoryFactory(factory))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}
	// Clean up the memory file this example creates so re-running it
	// starts from a blank history each time.
	defer os.Remove(".simon_chats/main-" + path)

	// First Run establishes context ("My name is Ada") that gets written to
	// the JSON-backed Memory.
	if _, err := session.Run(context.Background(), "My name is Ada"); err != nil {
		log.Fatal(err)
	}
	// Second Run on the same session sees the first exchange in its
	// history, proving memory survived between calls.
	resp, err := session.Run(context.Background(), "What did I just tell you?")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.Text)
}
