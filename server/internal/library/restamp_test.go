package library_test

import (
	"context"
	"testing"

	"magpie/internal/library/api"

	"gorm.io/gorm"
)

// unstampETags clears every stored ETag, as migration 00005 does.
func (f *importFixture) unstampETags(t *testing.T) {
	t.Helper()
	err := f.db.Write(context.Background(), func(tx *gorm.DB) error {
		return tx.Exec("UPDATE library_media_files SET etag = ''").Error
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *importFixture) etag(t *testing.T, file api.MediaFile) string {
	t.Helper()
	source, err := f.repo.SourceForProcessing(context.Background(), file.ID)
	if err != nil {
		t.Fatal(err)
	}
	return source.ETag
}

// Seed library #23.
func TestAFileWithoutAnETagIsStampedFromDiskAndSwapsIn(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	f.unstampETags(t)

	if err := f.service.RestampETags(context.Background()); err != nil {
		t.Fatal(err)
	}

	if f.etag(t, file) == "" {
		t.Fatal("the file still has no ETag")
	}
	f.verify(t, f.stage(t, file))
	if got := f.rejections(t); len(got) != 0 {
		t.Fatalf("rejections = %+v, want none", got)
	}
}

func TestRestampingKeepsStoredETags(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	stored := f.etag(t, file)

	if err := f.service.RestampETags(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := f.etag(t, file); got != stored {
		t.Fatalf("ETag = %q, want the stored %q", got, stored)
	}
}

func TestRestampingSkipsAFileGoneFromDisk(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	file := f.importMP4(t, "h264 original")
	if err := f.files.Remove(file.Path); err != nil {
		t.Fatal(err)
	}
	f.unstampETags(t)

	if err := f.service.RestampETags(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := f.etag(t, file); got != "" {
		t.Fatalf("ETag = %q, want none for a missing file", got)
	}
}
