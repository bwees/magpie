package library

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"

	"github.com/mikestefanello/backlite"
)

const (
	importQueueName = "library.import"
	// Database errors and shutdowns retry through backlite; file errors retry on importRetries instead.
	importQueueAttempts = 3
	importQueueBackoff  = time.Minute
)

// importRetries are the waits before each retry of an import that hit a file error. Once they run out,
// the import fails and ImportFailed is published.
var importRetries = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

var errNotInJob = errors.New("library: a delayed retry needs a running job")

// importTask imports one download. It is keyed by the download: the imports row says whether it ran.
type importTask struct {
	Download domain.DownloadID `json:"download"`
	// Retries are the waits left before giving up.
	Retries []time.Duration `json:"retries"`
}

func newImportTask(id domain.DownloadID) importTask {
	return importTask{Download: id, Retries: importRetries}
}

func (importTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: importQueueName, MaxAttempts: importQueueAttempts, Backoff: importQueueBackoff}
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

func newImportQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task importTask) error {
		return s.runImport(ctx, task, enqueueFromRunningJob)
	})
}

// runImport imports a download. A problem the user must solve fails it at once; a file error is retried
// on the task's schedule, then fails it. A shutdown hands the error to backlite, which retries the task.
func (s *Service) runImport(ctx context.Context, task importTask, later delayedEnqueue) error {
	err := s.importDownload(ctx, task.Download)
	var problem *importProblem
	switch {
	case err == nil:
		return nil
	case errors.As(err, &problem):
		return s.failImport(ctx, task.Download, problem)
	case ctx.Err() != nil:
		return err
	case len(task.Retries) > 0:
		if err := s.noteAttempt(ctx, task.Download, err); err != nil {
			return err
		}
		return later(ctx, importTask{Download: task.Download, Retries: task.Retries[1:]}, task.Retries[0])
	}
	return s.failImport(ctx, task.Download, failed(err.Error(), nil))
}
