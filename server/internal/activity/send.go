package activity

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/mikestefanello/backlite"
)

const (
	sendQueueName = "activity.send"
	// Database errors retry through backlite; an unavailable service retries on sendRetryWaits instead.
	sendQueueAttempts = 3
	sendQueueBackoff  = time.Minute
)

// sendRetryWaits are the waits before each retry of an unavailable service: three attempts in all.
var sendRetryWaits = []time.Duration{time.Minute, 5 * time.Minute}

var errNotInJob = errors.New("activity: a delayed retry needs a running job")

// sendTask delivers one notice to one URL. It names the URL by ID, so the secret stays out of the queue.
type sendTask struct {
	URL     string  `json:"url"`
	Kind    string  `json:"kind"`
	Message Message `json:"message"`
	// Retries are the waits left before giving up.
	Retries []time.Duration `json:"retries"`
}

func newSendTask(url, kind string, m Message) sendTask {
	return sendTask{URL: url, Kind: kind, Message: m, Retries: slices.Clone(sendRetryWaits)}
}

func (sendTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: sendQueueName, MaxAttempts: sendQueueAttempts, Backoff: sendQueueBackoff}
}

// retried is the task for the next attempt.
func (t sendTask) retried() sendTask {
	t.Retries = t.Retries[1:]
	return t
}

// delayedEnqueue adds a task that runs after wait.
type delayedEnqueue func(ctx context.Context, task backlite.Task, wait time.Duration) error

// enqueueFromRunningJob uses the backlite client that is running the current job.
func enqueueFromRunningJob(ctx context.Context, task backlite.Task, wait time.Duration) error {
	client := backlite.FromContext(ctx)
	if client == nil {
		return errNotInJob
	}
	_, err := client.Add(task).Ctx(ctx).Wait(wait).Save()
	return err
}

func newSendQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task sendTask) error {
		return s.Deliver(ctx, task, enqueueFromRunningJob)
	})
}
