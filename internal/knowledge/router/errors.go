package router

import (
	"fmt"

	"simon-go/pkg/simonerr"
)

// wrapf builds a knowledge-domain error with a "knowledge router: <context>: <cause>"
// message, reusing pkg/simonerr so callers can still errors.Is against
// simonerr.ErrKnowledge / simonerr.ErrRuntime.
func wrapf(cause error, format string, args ...any) error {
	msg := "knowledge router: " + fmt.Sprintf(format, args...)
	return simonerr.NewKnowledgeError(msg, cause)
}
