// Command public_parallel_sessions demonstrates running multiple Sessions
// on one Runtime concurrently, and WithMaxConcurrentRuns throttling how
// many of those runs execute at once.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"simon-go/model"
	"simon-go/simon"
)

func main() {
	// WithMaxConcurrentRuns(2) caps the Runtime at 2 in-flight Run calls at
	// once; the other 3 of the 5 goroutines below block until a slot frees
	// up, rather than all 5 hitting the model concurrently.
	rt, err := simon.New(simon.WithModel(model.EchoModel{}), simon.WithMaxConcurrentRuns(2))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]string, 0, 5)

	// Launch 5 independent sessions concurrently, each on its own
	// goroutine. Sessions are independent conversation state, so they can
	// safely share one Runtime.
	for i := 0; i < 5; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := rt.NewSession(fmt.Sprintf("session-%d", i))
			if err != nil {
				log.Println("session", i, "error:", err)
				return
			}
			resp, err := session.Run(context.Background(), fmt.Sprintf("task %d", i))
			if err != nil {
				log.Println("session", i, "run error:", err)
				return
			}
			// results is shared across goroutines, so appends must be
			// guarded by a mutex.
			mu.Lock()
			results = append(results, resp.Text)
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, r := range results {
		fmt.Println(r)
	}
}
