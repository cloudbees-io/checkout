package helper

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	format "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func uniqueId(serverURL string) string {
	h := sha256.New()
	h.Write([]byte(serverURL))
	bs := h.Sum(nil)
	return fmt.Sprintf("%x", bs)[0:16]
}

func copyFileHelper(dst string, src string) (err error) {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err2 := f.Close()
		if err2 != nil && err == nil {
			err = err2
		}
	}(s)

	if stat, err := os.Stat(dst); err == nil {
		// set up to force delete
		if err := os.Chmod(dst, stat.Mode()|0222); err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil {
			return err
		}
	}

	// Create the destination file with default permission
	d, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0555)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err2 := f.Close()
		if err2 != nil && err == nil {
			err = err2
		}
	}(d)

	_, err = io.Copy(d, s)
	return err
}

func noOpClean() error {
	return nil
}

func removeFilesClean(files ...string) func() error {
	return func() error {
		fmt.Println("🔄 Removing credentials helper ...")
		var errs []error
		for _, f := range files {
			if stat, err := os.Stat(f); err == nil {
				// set up to force delete
				if err := os.Chmod(f, stat.Mode()|0222); err != nil {
					errs = append(errs, err)
				}
				if err := os.Remove(f); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		fmt.Println("✅ Credentials helper removed")
		return nil
	}
}

func InstallHelperFor(serverURL string, options map[string][]string) (string, func() error, error) {
	homePath := os.Getenv("HOME")
	actionPath := filepath.Join(homePath, ".cloudbees-checkout", uniqueId(serverURL))

	fmt.Println("🔄 Installing credentials helper ...")

	self, err := os.Executable()
	if err != nil {
		return "", noOpClean, err
	}

	if err := os.MkdirAll(actionPath, os.ModePerm); err != nil {
		return "", noOpClean, err
	}

	helperExecutable := filepath.Join(actionPath, "git-credential-helper")
	if a, err := filepath.Abs(helperExecutable); err != nil {
		helperExecutable = a
	}

	err = copyFileHelper(helperExecutable, self)
	if err != nil {
		return "", noOpClean, err
	}

	fmt.Println("✅ Credentials helper installed")

	helperConfig := &format.Config{}
	helperConfigFile := helperExecutable + ".cfg"

	ep, err := transport.NewEndpoint(serverURL)
	if err != nil {
		return "", removeFilesClean(helperExecutable), err
	}

	sec := helperConfig.Section(ep.Protocol)

	s := sec.Subsection(strings.TrimPrefix(ep.String(), ep.Protocol+":"))

	for k, v := range options {
		s.SetOption(k, v...)
	}

	var b bytes.Buffer
	if err := format.NewEncoder(&b).Encode(helperConfig); err != nil {
		return "", removeFilesClean(helperExecutable), err
	}
	if err := os.WriteFile(helperConfigFile, b.Bytes(), 0666); err != nil {
		return "", removeFilesClean(helperExecutable), err
	}

	return fmt.Sprintf("%s credential-helper --config-file %s", helperExecutable, helperConfigFile),
		removeFilesClean(helperExecutable, helperConfigFile), nil
}
