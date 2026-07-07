// Package pipeline holds end-to-end tests of the activity pipeline
// (sensor -> bus -> store -> semantic -> activity -> habit) wired together
// from the individually-tested packages in internal/events, internal/privacy,
// internal/semantic, internal/activity, and internal/habits. It has no
// production code of its own.
package pipeline
