// package mounts provides abstracted filemounts to use as fs.FS filesystems in a
// program. The package allows either the embedded file system to be used or, when
// specified, the path to a directory on disk. The package takes care of mounting the
// filesystem at the same level, something that does not happen by default.
package internal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileMount is a mount that may be mounted by either an embedded fs.FS or a filePath.
type FileMount struct {
	MountName string
	fs.FS
}

// String describes a fileMount as a list of files and directories indented by the file
// or directory level.
func (fm FileMount) String() string {
	o := fmt.Sprintf("fileMount %q:\n", fm.MountName)
	s, _ := PrintFS(fm.FS)
	return o + s
}

// ErrInvalidPath reports an invalid mount name.
type ErrInvalidPath struct {
	mountName string
}

// Error fulfills the Error interface requirement for ErrInvalidPath.
func (e ErrInvalidPath) Error() string {
	tpl := strings.Join([]string{
		"mount name %q is not a valid fs.ValidPath path",
		"see https://pkg.go.dev/io/fs#ValidPath for more information.",
	}, "\n")
	return fmt.Sprintf(tpl, e.mountName)
}

// NewFileMount takes an embedded fs.FS or a path to a directory. If the path to the
// directory is "", the embedded fs is used, otherwise the directory is used. Note that
// the MountName parameter is used to name the mount of an fs.FS to make it work like an
// os.DirFS.
//
// In other words, given a directory "templates" and a go embed fs.FS called templateFS
// such as:
//
//	//go:embed "path/templates"
//	var templatesFS fs.FS
//
// and a `NewFileMount` invocation such as
//
//	NewFileMount("path/templates", templatesFS, "")
//
// will mount the embedded fs.FS at the equivalent of "path/templates/" rather than ".",
// giving the impression of moving the fs level up one.
//
// On the other hand an invocation as follows:
//
//	NewFileMount("path/templates", templatesFS, "path/templates")
//
// will mount the filesystem location "templates" to "templates/".
func NewFileMount(mountName string, embeddedFS fs.FS, dirPath string) (*FileMount, error) {

	if mountName == "" {
		return nil, errors.New("no mount name provided for new file mount")
	}
	if !fs.ValidPath(mountName) {
		return nil, ErrInvalidPath{mountName}
	}

	// If a directory is not provided, use the embedded fs, but mounted at the
	// subdirectory provided at MountName.
	if dirPath == "" {
		subFS, err := fs.Sub(embeddedFS, mountName)
		if err != nil {
			return nil, fmt.Errorf("could not sub-mount embedded fs at %q: %v", mountName, err)
		}
		return &FileMount{
			mountName,
			subFS,
		}, nil
	}

	s, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("new mount at %q error: %s", dirPath, err)
	}
	if !s.IsDir() {
		return nil, fmt.Errorf("new mount at %q is not a directory", dirPath)
	}

	dirFS := os.DirFS(dirPath)
	return &FileMount{
		mountName,
		dirFS,
	}, nil
}

// Materialize outputs the data in fm.FS recursively to the filesystem starting at
// root plus mount name. For example running:
//
//	nf, _ := NewFileMount("path/templates", templatesFS, "")
//	_ = nf.Materialize("/tmp/")
//
// will create the contents of templatesFS in "/tmp/path/templates/". Root must be a
// directory and the constructed output path must not exist on the system.
func (fm *FileMount) Materialize(root string) error {

	checkIsDir := func(path string) error {
		s, err := os.Stat(path)
		if err != nil {
			var fspe *fs.PathError // https://go.dev/wiki/ErrorValueFAQ
			if errors.As(err, &fspe) {
				return errors.New("was not found")
			}
			return err
		}
		if !s.IsDir() {
			return errors.New("is not a directory")
		}
		return nil
	}

	// Check the root exists and intended file path does not already exist.
	if err := checkIsDir(root); err != nil {
		return fmt.Errorf("materialize root %q invalid: %v", root, err)
	}

	// Make a dir named fm.MountName at root. Since fm.MountName may be a composite
	// path, MkdirAll is used to create it.
	mountRoot := filepath.Join(root, fm.MountName)
	if _, err := os.Stat(mountRoot); !os.IsNotExist(err) {
		return fmt.Errorf("materialization path %q already exists", mountRoot)
	}
	if err := os.MkdirAll(mountRoot, 0755); err != nil {
		return fmt.Errorf("could not create mount root %q: %v", mountRoot, err)
	}

	// Recurse, writing files and making directories from the fs.FS.
	err := fs.WalkDir(fm.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // error propogation
		}
		fullPath := filepath.Join(mountRoot, path)

		if d.IsDir() {
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				return fmt.Errorf("could not make dir %q, %v", fullPath, err)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			// not a regular file
			return nil
		}

		data, err := fs.ReadFile(fm.FS, path)
		if err != nil {
			return fmt.Errorf("could not read %q from mount %s: %v", path, fm.MountName, err)
		}
		err = os.WriteFile(fullPath, data, 0644)
		if err != nil {
			return fmt.Errorf("could not write %q at %q from mount %s: %v", path, fullPath, fm.MountName, err)
		}
		return nil
	})
	return err
}

// PrintFS makes structured print output from an fs.FS.
func PrintFS(thisFS fs.FS) (string, error) {
	var printOutput strings.Builder
	var topSeen bool
	tpl := "%s[%s] %s%s (%s)\n"

	err := fs.WalkDir(thisFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propogate
		}
		if !topSeen { // verbatim root as "[d] ./ (./)"
			_, err = printOutput.WriteString(fmt.Sprintf(tpl, "\n", "d", ".", "/", "."))
			if err != nil {
				return fmt.Errorf("printOutput error: %v", err)
			}
			topSeen = true
			return nil
		}
		depth := strings.Count(path, string(os.PathSeparator))
		indent := strings.Repeat("  ", depth)
		typer := "f"
		name := d.Name()
		slash := " "
		if d.IsDir() {
			slash = string(os.PathSeparator)
			typer = "d"
		}
		_, err = printOutput.WriteString(fmt.Sprintf(tpl, indent, typer, name, slash, path))
		return err
	})
	return printOutput.String(), err
}
