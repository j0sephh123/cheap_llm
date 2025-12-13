package main

import (
	"os/exec"

	"github.com/atotto/clipboard"
)

// CopyToClipboard copies text to the system clipboard
// It tries atotto/clipboard first, then falls back to platform-specific tools
func CopyToClipboard(text string) error {
	// Try atotto/clipboard first
	err := clipboard.WriteAll(text)
	if err == nil {
		return nil
	}

	// Fallback to pbcopy (macOS)
	if pbcopyPath, err := exec.LookPath("pbcopy"); err == nil {
		cmd := exec.Command(pbcopyPath)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		if _, err := stdin.Write([]byte(text)); err != nil {
			return err
		}

		if err := stdin.Close(); err != nil {
			return err
		}

		return cmd.Wait()
	}

	// Fallback to xclip (Linux)
	if xclipPath, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(xclipPath, "-selection", "clipboard")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		if _, err := stdin.Write([]byte(text)); err != nil {
			return err
		}

		if err := stdin.Close(); err != nil {
			return err
		}

		return cmd.Wait()
	}

	// Fallback to xsel (Linux)
	if xselPath, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(xselPath, "--clipboard", "--input")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}

		if _, err := stdin.Write([]byte(text)); err != nil {
			return err
		}

		if err := stdin.Close(); err != nil {
			return err
		}

		return cmd.Wait()
	}

	// Return original error if no fallback worked
	return err
}
