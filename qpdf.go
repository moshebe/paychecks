package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var errInvalidPassword = fmt.Errorf("invalid password")

func qpdfDecrypt(filename, decFilename, password string) error {
	cmd := exec.Command("qpdf", "--decrypt", "--password="+password, filename, decFilename)
	out, err := cmd.CombinedOutput()
	output := string(out)
	if strings.Contains(output, "invalid password") {
		return errInvalidPassword
	}
	if err != nil {
		return err
	}
	return nil
}

func tryDecrypt(filename, decFilename string, passwords []string) error {
	for _, p := range passwords {
		err := qpdfDecrypt(filename, decFilename, p)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errInvalidPassword) {
			return err
		}
	}
	return errInvalidPassword
}

func decrypt(filename string, passwords []string) error {
	decFilename := filename + ".dec"
	err := tryDecrypt(filename, decFilename, passwords)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	err = os.Rename(decFilename, filename)
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
