package internal

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

//go:embed testdata
var testdata embed.FS

//go:embed testdata/dirA
var testdataDirA embed.FS

func TestMounts(t *testing.T) {

	tests := []struct {
		name       string
		mountName  string
		embeddedFS fs.FS
		dirPath    string
		dirToStat  string
		wantErr    error
	}{
		{
			name:       "embedded fs mount",
			mountName:  "testdata",
			embeddedFS: testdata,
			dirPath:    "",
			dirToStat:  "dirA/dirB",
		},
		{
			name:       "directory fs mount",
			mountName:  "testdata",
			embeddedFS: testdata,
			dirPath:    "./testdata",
			dirToStat:  "dirA/dirB",
		},
		{
			name:       "directory fs mount fail",
			mountName:  "testdata",
			embeddedFS: testdata,
			dirPath:    "./doesNotExist",
			wantErr:    errors.New(`new mount at "./doesNotExist"`),
		},
		{
			name:       "embedded fs mount for dirA",
			mountName:  "testdata/dirA",
			embeddedFS: testdataDirA,
			dirPath:    "",
			dirToStat:  "dirB",
		},
		{
			name:       "directory fs mount for dirA",
			mountName:  "testdata/dirA",
			embeddedFS: testdataDirA,
			dirPath:    "testdata/dirA",
			dirToStat:  "dirB",
		},
		// fs.ValidPath docs
		//
		// Path names passed to open are UTF-8-encoded, unrooted, slash-separated sequences
		// of path elements, like “x/y/z”. Path names must not contain an element that is
		// “.” or “..” or the empty string, except for the special case that the name "."
		// may be used for the root directory. Paths must not start or end with a slash:
		// “/x” and “x/” are invalid.
		//
		// Note that paths are slash-separated on all systems, even Windows. Paths
		// containing other characters such as backslash and colon are accepted as valid,
		// but those characters must never be interpreted by an FS implementation as path
		// element separators.
		{
			name:       "invalid mount name",
			mountName:  `/dev/null`,
			embeddedFS: testdata,
			dirPath:    "testdata",
			dirToStat:  "",
			wantErr:    ErrInvalidPath{`/dev/null`},
		},
		{
			name:       "another invalid mount name",
			mountName:  `testdata/`,
			embeddedFS: testdata,
			dirPath:    "",
			dirToStat:  "",
			wantErr:    ErrInvalidPath{`testdata/`},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			testDir := t.TempDir()
			// uncomment to inspect directory
			// testDir, err := os.MkdirTemp("", "mount_*")
			// if err != nil {
			// 	t.Fatal(err)
			// }

			fm, err := NewFileMount(tt.mountName, tt.embeddedFS, tt.dirPath)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got none (mount %s)", tt.wantErr, fm.MountName)
				}

				// Check if the error is of the ErrInvalidPath type.
				var eip ErrInvalidPath
				if errors.As(tt.wantErr, &eip) {
					if !errors.As(err, &eip) {
						t.Errorf("expected ErrInvalidPath error, got %v", err)
					}
					return
				}
				// Otherwise check the error string.
				if got, want := err.Error(), tt.wantErr.Error(); !strings.Contains(got, want) {
					t.Errorf("error got %q want substring %q", got, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			stat, err := fs.Stat(fm.FS, tt.dirToStat)
			if err != nil {
				t.Fatalf("could not find dir 'dirB' in 'dirA' at top level of fs: %v", err)
			}
			if !stat.IsDir() {
				t.Errorf("dir 'dirB' in 'dirA' of fs is not a dir: %v", stat.Name())
			}
			err = fm.Materialize(testDir)
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}

			// Given a target of /tmp the materialized output is put in (for example)
			// /tmp/testdata/. To compensate for this the top level of the materialized
			// output is popped.

			matFS := os.DirFS(testDir)
			materializedFS, err := fs.Sub(matFS, tt.mountName)
			if err != nil {
				t.Fatalf("could not submount materialized dir")
			}
			materializedFSAsString, err := PrintFS(materializedFS)
			if err != nil {
				t.Fatal(err)
			}

			mountFSAsString, err := PrintFS(fm.FS)
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(materializedFSAsString, mountFSAsString); diff != "" {
				t.Errorf("unexpected difference between materialization and mount:\n%s", diff)
			}

		})
	}
}
