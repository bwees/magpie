package whisper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

const (
	// DefaultModelsURL serves the ggml models whisper.cpp's own download script fetches.
	DefaultModelsURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
	// DefaultVADURL is the Silero VAD model whisper.cpp's README uses.
	DefaultVADURL = "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin"

	modelPrefix    = "ggml-"
	modelExtension = ".bin"
	partialSuffix  = ".part"
	modelsDirPerm  = 0o750
)

var (
	ErrUnknownModel = errors.New("whisper: not a whisper.cpp model name")
	modelName       = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)
)

// Models keeps whisper.cpp's model files in a directory, downloading each one the first time a job
// needs it. A file already in the directory is used as is, so an offline agent can be given its models.
type Models struct {
	Dir       string
	ModelsURL string
	VADURL    string
	Client    *http.Client

	// mu keeps two jobs from downloading the same file at once.
	mu sync.Mutex
}

func NewModels(dir string) *Models {
	return &Models{Dir: dir, ModelsURL: DefaultModelsURL, VADURL: DefaultVADURL, Client: http.DefaultClient}
}

// Model returns the path of a model such as large-v3-turbo.
func (m *Models) Model(ctx context.Context, name string) (string, error) {
	if !modelName.MatchString(name) {
		return "", fmt.Errorf("%w: %q", ErrUnknownModel, name)
	}
	file := modelPrefix + name + modelExtension
	return m.ensure(ctx, file, m.ModelsURL+"/"+file)
}

// VAD returns the path of the Silero voice-activity model.
func (m *Models) VAD(ctx context.Context) (string, error) {
	return m.ensure(ctx, filepath.Base(m.VADURL), m.VADURL)
}

func (m *Models) ensure(ctx context.Context, file, url string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := filepath.Join(m.Dir, file)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(m.Dir, modelsDirPerm); err != nil {
		return "", err
	}
	if err := m.download(ctx, url, path+partialSuffix); err != nil {
		return "", fmt.Errorf("downloading %s: %w", file, err)
	}
	return path, os.Rename(path+partialSuffix, path)
}

func (m *Models) download(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	return errors.Join(err, out.Close())
}
