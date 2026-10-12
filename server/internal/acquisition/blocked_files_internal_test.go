package acquisition

import "testing"

func TestBlockedPatternsMatchFileNamesOnlyAndIgnoreCase(t *testing.T) {
	blocked, err := compileBlockedPatterns(append(DefaultBlockedPatterns, RegexPatternPrefix+`^setup\.`))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"Movie.mkv":                  false,
		"Movie.EXE":                  true,
		"Show.S01E01.rar":            true,
		"Show.S01E01.r07":            true,
		"Show.S01E01.r7":             false,
		"extras/setup.txt":           true,
		"Movie.zip.folder/Movie.mkv": false,
		`Sub\Movie.Bat`:              true,
	} {
		if _, got := blocked.firstMatch([]string{path}); got != want {
			t.Errorf("%s blocked = %v, want %v", path, got, want)
		}
	}
}

func TestTheCheckCanBeTurnedOff(t *testing.T) {
	off := DownloadOptions{BlockedFilesEnabled: false, BlockedPatterns: DefaultBlockedPatterns}
	blocked, err := off.blockedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if file, found := blocked.firstMatch([]string{"Movie.exe"}); found {
		t.Errorf("%s blocked with the check off", file)
	}
}
