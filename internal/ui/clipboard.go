package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// ClipboardImage writes any image on the system clipboard to a temporary file
// and returns its path. Terminals deliver a paste as text, so an image on the
// clipboard never reaches the program as keystrokes — it has to be asked for.
func ClipboardImage() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}

	target := filepath.Join(
		os.TempDir(),
		"bb-tui-paste-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".png",
	)

	// osascript is always present on macOS; pngpaste is not. Asking AppleScript
	// for «class PNGf» fails cleanly when the clipboard holds anything else.
	script := `set outFile to (POSIX file "` + target + `")
set imageData to the clipboard as «class PNGf»
set fileRef to open for access outFile with write permission
set eof fileRef to 0
write imageData to fileRef
close access fileRef`

	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		_ = os.Remove(target)
		return "", false
	}
	info, err := os.Stat(target)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(target)
		return "", false
	}
	return target, true
}
